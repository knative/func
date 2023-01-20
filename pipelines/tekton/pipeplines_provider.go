package tekton

import (
	"archive/tar"
	"context"
	goErrors "errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/tektoncd/cli/pkg/pipelinerun"
	"github.com/tektoncd/cli/pkg/taskrun"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	fn "knative.dev/func"
	"knative.dev/func/docker"
	"knative.dev/func/k8s"
	fnlabels "knative.dev/func/k8s/labels"
	"knative.dev/func/knative"
	"knative.dev/pkg/apis"
)

type PipelineDecorator interface {
	UpdateLabels(fn.Function, map[string]string) map[string]string
}

type Opt func(*PipelinesProvider)

type PipelinesProvider struct {
	// namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, pipeline creation/run will commence to the configured namespace.
	namespace           string
	verbose             bool
	progressListener    fn.ProgressListener
	credentialsProvider docker.CredentialsProvider
	decorator           PipelineDecorator
}

func WithNamespace(namespace string) Opt {
	return func(pp *PipelinesProvider) {
		pp.namespace = namespace
	}
}

func WithProgressListener(pl fn.ProgressListener) Opt {
	return func(pp *PipelinesProvider) {
		pp.progressListener = pl
	}
}

func WithCredentialsProvider(credentialsProvider docker.CredentialsProvider) Opt {
	return func(pp *PipelinesProvider) {
		pp.credentialsProvider = credentialsProvider
	}
}

func WithVerbose(verbose bool) Opt {
	return func(pp *PipelinesProvider) {
		pp.verbose = verbose
	}
}

func WithPipelineDecorator(decorator PipelineDecorator) Opt {
	return func(pp *PipelinesProvider) {
		pp.decorator = decorator
	}
}

func NewPipelinesProvider(opts ...Opt) *PipelinesProvider {
	pp := &PipelinesProvider{}

	for _, opt := range opts {
		opt(pp)
	}

	return pp
}

// Run creates a Tekton Pipeline and all necessary resources (PVCs, Secrets, SAs,...) for the input Function.
// It ensures that all needed resources are present on the cluster so the PipelineRun can be initialized.
// After the PipelineRun is being initialized, the progress of the PipelineRun is being watched and printed to the output.
func (pp *PipelinesProvider) Run(ctx context.Context, f fn.Function) error {
	pp.progressListener.Increment("Creating Pipeline resources")

	if err := validatePipeline(f); err != nil {
		return err
	}

	client, namespace, err := NewTektonClientAndResolvedNamespace(pp.namespace)
	if err != nil {
		return err
	}
	pp.namespace = namespace

	// let's specify labels that will be applied to every resource that is created for a Pipeline
	labels, err := f.LabelsMap()
	if err != nil {
		return err
	}
	if pp.decorator != nil {
		labels = pp.decorator.UpdateLabels(f, labels)
	}

	err = createPipelinePersistentVolumeClaim(ctx, f, pp.namespace, labels, DefaultPersistentVolumeClaimSize)
	if err != nil {
		return err
	}

	if f.Build.Git.URL == "" {
		// Use direct upload to PVC if Git is not set up.
		content := sourcesAsTarStream(f)
		defer content.Close()
		err = k8s.UploadToVolume(ctx, content, getPipelinePvcName(f), pp.namespace)
		if err != nil {
			return fmt.Errorf("cannot upload sources to the PVC: %w", err)
		}
	}

	_, err = client.Pipelines(pp.namespace).Create(ctx, generatePipeline(f, labels), metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			if errors.IsNotFound(err) {
				return fmt.Errorf("problem creating pipeline, missing tekton?: %v", err)
			}
			return fmt.Errorf("problem creating pipeline: %v", err)
		}
	}

	registry, err := docker.GetRegistry(f.Image)
	if err != nil {
		return fmt.Errorf("problem in resolving image registry name: %v", err)
	}

	creds, err := pp.credentialsProvider(ctx, f.Image)
	if err != nil {
		return err
	}

	if registry == name.DefaultRegistry {
		registry = authn.DefaultAuthKey
	}

	err = k8s.EnsureDockerRegistrySecretExist(ctx, getPipelineSecretName(f), pp.namespace, labels, f.Deploy.Annotations, creds.Username, creds.Password, registry)
	if err != nil {
		return fmt.Errorf("problem in creating secret: %v", err)
	}

	pr, err := client.PipelineRuns(pp.namespace).Create(ctx, generatePipelineRun(f, labels), metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("problem in creating pipeline run: %v", err)
	}

	err = pp.watchPipelineRunProgress(ctx, pr)
	if err != nil {
		if !goErrors.Is(err, context.Canceled) {
			return fmt.Errorf("problem in watching started pipeline run: %v", err)
		}
		// TODO replace deletion with pipeline-run cancellation
		_ = client.PipelineRuns(pp.namespace).Delete(context.TODO(), pr.Name, metav1.DeleteOptions{})
		return fmt.Errorf("pipeline run cancelled: %w", context.Canceled)
	}

	pr, err = client.PipelineRuns(pp.namespace).Get(ctx, pr.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("problem in retriving pipeline run status: %v", err)
	}

	if pr.Status.GetCondition(apis.ConditionSucceeded).Status == corev1.ConditionFalse {
		message := getFailedPipelineRunLog(ctx, pr, pp.namespace)
		return fmt.Errorf("function pipeline run has failed with message: \n\n%s", message)
	}

	kClient, err := knative.NewServingClient(pp.namespace)
	if err != nil {
		return fmt.Errorf("problem in retrieving status of deployed function: %v", err)
	}

	ksvc, err := kClient.GetService(ctx, f.Name)
	if err != nil {
		return fmt.Errorf("problem in retrieving status of deployed function: %v", err)
	}

	if ksvc.Generation == 1 {
		pp.progressListener.Increment(fmt.Sprintf("✅ Function deployed in namespace %q and exposed at URL: \n   %s", ksvc.Namespace, ksvc.Status.URL.String()))
	} else {
		pp.progressListener.Increment(fmt.Sprintf("✅ Function updated in namespace %q and exposed at URL: \n   %s", ksvc.Namespace, ksvc.Status.URL.String()))
	}

	return nil
}

// Creates tar stream with the function sources as they were in "./source" directory.
func sourcesAsTarStream(f fn.Function) *io.PipeReader {
	ignored := func(p string) bool { return strings.HasPrefix(p, ".git") }
	if gi, err := gitignore.CompileIgnoreFile(filepath.Join(f.Root, ".gitignore")); err == nil {
		ignored = func(p string) bool {
			if strings.HasPrefix(p, ".git") {
				return true
			}
			return gi.MatchesPath(p)
		}
	}

	pr, pw := io.Pipe()

	const up = ".." + string(os.PathSeparator)
	go func() {
		tw := tar.NewWriter(pw)
		err := filepath.Walk(f.Root, func(p string, fi fs.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("error traversing function directory: %w", err)
			}

			relp, err := filepath.Rel(f.Root, p)
			if err != nil {
				return fmt.Errorf("cannot get relative path: %w", err)
			}

			if relp == "." {
				return nil
			}

			if ignored(relp) {
				return nil
			}

			lnk := ""
			if fi.Mode()&fs.ModeSymlink != 0 {
				lnk, err = os.Readlink(p)
				if err != nil {
					return fmt.Errorf("cannot read link: %w", err)
				}
				if filepath.IsAbs(lnk) {
					lnk, err = filepath.Rel(f.Root, lnk)
					if err != nil {
						return fmt.Errorf("cannot get relative path for symlink: %w", err)
					}
				}
				if strings.HasPrefix(lnk, up) || lnk == ".." {
					return fmt.Errorf("link %q points outside source root", p)
				}
			}

			hdr, err := tar.FileInfoHeader(fi, lnk)
			if err != nil {
				return fmt.Errorf("cannot create a tar header: %w", err)
			}
			// "source" is expected path in workspace pvc
			hdr.Name = path.Join("source", filepath.ToSlash(relp))

			err = tw.WriteHeader(hdr)
			if err != nil {
				return fmt.Errorf("cannot write header to tar stream: %w", err)
			}

			if fi.Mode().IsRegular() {
				var file io.ReadCloser
				file, err = os.Open(p)
				if err != nil {
					return fmt.Errorf("cannot open source file: %w", err)
				}
				defer file.Close()
				_, err = io.Copy(tw, file)
				if err != nil {
					return fmt.Errorf("cannot copy source file content: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("error while creating tar stream from sources: %w", err))
		} else {
			_ = tw.Close()
			_ = pw.Close()
		}
	}()
	return pr
}

func (pp *PipelinesProvider) Remove(ctx context.Context, f fn.Function) error {

	l := k8slabels.SelectorFromSet(k8slabels.Set(map[string]string{fnlabels.FunctionNameKey: f.Name}))
	listOptions := metav1.ListOptions{
		LabelSelector: l.String(),
	}

	// let's try to delete all resources in parallel, so the operation doesn't take long
	wg := sync.WaitGroup{}
	deleteFunctions := []func(context.Context, string, metav1.ListOptions) error{
		deletePipelines,
		deletePipelineRuns,
		k8s.DeleteSecrets,
		k8s.DeletePersistentVolumeClaims,
	}

	wg.Add(len(deleteFunctions))
	errChan := make(chan error, len(deleteFunctions))

	for i := range deleteFunctions {
		df := deleteFunctions[i]
		go func() {
			defer wg.Done()
			err := df(ctx, pp.namespace, listOptions)
			if err != nil && !errors.IsNotFound(err) && !errors.IsForbidden(err) {
				errChan <- err
			}
		}()
	}
	wg.Wait()
	close(errChan)

	// collect all errors and print them
	var err error
	errMsg := ""
	anyError := false
	for e := range errChan {
		if !anyError {
			anyError = true
			errMsg = "error deleting resources:"
		}
		errMsg += fmt.Sprintf("\n %v", e)
	}

	if anyError {
		err = fmt.Errorf("%s", errMsg)
	}

	return err
}

// watchPipelineRunProgress watches the progress of the input PipelineRun
// and prints detailed description of the currently executed Tekton Task.
func (pp *PipelinesProvider) watchPipelineRunProgress(ctx context.Context, pr *v1beta1.PipelineRun) error {
	taskProgressMsg := map[string]string{
		taskNameFetchSources: "Fetching git repository with the function source code",
		taskNameBuild:        "Building function image on the cluster",
		taskNameDeploy:       "Deploying function to the cluster",
	}

	clientset, err := NewTektonClientset()
	if err != nil {
		return err
	}

	prTracker := pipelinerun.NewTracker(pr.Name, pp.namespace, clientset)
	trChannel := prTracker.Monitor([]string{})
	ctxDone := ctx.Done()
	wg := sync.WaitGroup{}
out:
	for {
		var trs []taskrun.Run
		var ok bool

		select {
		case trs, ok = <-trChannel:
			if !ok {
				break out
			}
		case <-ctxDone:
			err = ctx.Err()
			break out
		}

		wg.Add(len(trs))

		for _, run := range trs {
			go func(tr taskrun.Run) {
				defer wg.Done()

				// let's print a Task name, if we don't have a proper description of the Task
				taskDescription := tr.Task
				if val, ok := taskProgressMsg[tr.Task]; ok {
					taskDescription = val
				}
				pp.progressListener.Increment(fmt.Sprintf("Running Pipeline Task: %s", taskDescription))

			}(run)
		}
	}
	wg.Wait()

	return err
}

// getFailedPipelineRunLog returns log message for a failed PipelineRun,
// returns log from a container where the failing TaskRun is running, if available.
func getFailedPipelineRunLog(ctx context.Context, pr *v1beta1.PipelineRun, namespace string) string {
	// Reason "Failed" usually means there is a specific failure in some step,
	// let's find the failed step and try to get log directly from the container.
	// If we are not able to get the container's log, we return the generic message from the PipelineRun.Status.
	message := pr.Status.GetCondition(apis.ConditionSucceeded).Message
	if pr.Status.GetCondition(apis.ConditionSucceeded).Reason == "Failed" {
		for _, t := range pr.Status.TaskRuns {
			if t.Status.GetCondition(apis.ConditionSucceeded).Status == corev1.ConditionFalse {
				for _, s := range t.Status.Steps {
					// let's try to print logs of the first unsuccessful step
					if s.Terminated != nil && s.Terminated.ExitCode != 0 {
						podLogs, err := k8s.GetPodLogs(ctx, namespace, t.Status.PodName, s.ContainerName)
						if err == nil {
							return podLogs
						}
						return message
					}
				}

			}
		}
	}

	return message
}

// allows simple mocking in unit tests, use with caution regarding concurrency
var createPersistentVolumeClaim = k8s.CreatePersistentVolumeClaim

func createPipelinePersistentVolumeClaim(ctx context.Context, f fn.Function, namespace string, labels map[string]string, size int64) error {
	err := createPersistentVolumeClaim(ctx, getPipelinePvcName(f), namespace, labels, f.Deploy.Annotations, corev1.ReadWriteOnce, *resource.NewQuantity(size, resource.DecimalSI))
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("problem creating persistent volume claim: %v", err)
	}
	return nil
}
