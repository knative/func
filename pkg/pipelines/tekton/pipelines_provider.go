package tekton

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/tektoncd/cli/pkg/pipelinerun"
	"github.com/tektoncd/cli/pkg/taskrun"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineClient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	fnlabels "knative.dev/func/pkg/k8s/labels"
	"knative.dev/func/pkg/knative"
	"knative.dev/pkg/apis"
)

// DefaultNamespace is the kubernetes default namespace
const DefaultNamespace = "default"

// DefaultPersistentVolumeClaimSize to allocate for the function.
var DefaultPersistentVolumeClaimSize = resource.MustParse("256Mi")

type PipelineDecorator interface {
	UpdateLabels(fn.Function, map[string]string) map[string]string
}

type Opt func(*PipelinesProvider)

type pacURLCallback = func() (string, error)

type PipelinesProvider struct {
	verbose             bool
	getPacURL           pacURLCallback
	credentialsProvider docker.CredentialsProvider
	decorator           PipelineDecorator
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

func WithPacURLCallback(getPacURL pacURLCallback) Opt {
	return func(pp *PipelinesProvider) {
		pp.getPacURL = getPacURL
	}
}

func NewPipelinesProvider(opts ...Opt) *PipelinesProvider {
	pp := &PipelinesProvider{
		getPacURL: func() (string, error) {
			var url string
			e := survey.AskOne(&survey.Input{
				Message: "Please enter your Pipelines As Code controller public route URL: ",
			}, &url, survey.WithValidator(survey.Required))
			return url, e
		},
	}

	for _, opt := range opts {
		opt(pp)
	}

	return pp
}

// Run a remote build by creating all necessary resources (PVCs, secrets,
// SAs, etc) specified by the given Function before generating a pipeline
// definition, sending it to the cluster to be run via Tekton.
// Progress is by default piped to stdtout.
// Returned is the final url, and the input Function with the final results of the run populated
// (f.Deploy.Image and f.Deploy.Namespace) or an error.
func (pp *PipelinesProvider) Run(ctx context.Context, f fn.Function) (string, fn.Function, error) {
	var err error

	// Checks builder and registry:
	if err = validatePipeline(f); err != nil {
		return "", f, err
	}

	// Namespace is either a new namespace, specified as f.Namespace, or
	// the currently deployed namespace, recorded on f.Deploy.Namespace.
	// If neither exist, an error is returned (namespace is required)
	namespace := f.Namespace
	if namespace == "" {
		namespace = f.Deploy.Namespace
	}
	if namespace == "" {
		return "", f, fn.ErrNamespaceRequired
	}
	f.Deploy.Namespace = namespace

	// Image is either an explicit image indicated with f.Image, or
	// generated from a name+registry
	image := f.Image
	if image == "" {
		image, err = f.ImageName()
		if err != nil {
			return "", f, err
		}
	}
	f.Deploy.Image = image

	// Client for the given namespace
	client, err := NewTektonClient(namespace)
	if err != nil {
		return "", f, err
	}

	// let's specify labels that will be applied to every resource that is created for a Pipeline
	labels, err := f.LabelsMap()
	if err != nil {
		return "", f, err
	}
	if pp.decorator != nil {
		labels = pp.decorator.UpdateLabels(f, labels)
	}

	err = createPipelinePersistentVolumeClaim(ctx, f, namespace, labels)
	if err != nil {
		return "", f, err
	}

	if f.Build.Git.URL == "" {
		// Use direct upload to PVC if Git is not set up.
		content := sourcesAsTarStream(f)
		defer content.Close()
		err = k8s.UploadToVolume(ctx, content, getPipelinePvcName(f), namespace)
		if err != nil {
			return "", f, fmt.Errorf("cannot upload sources to the PVC: %w", err)
		}
	}

	err = createAndApplyPipelineTemplate(f, namespace, labels)
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			if k8serrors.IsNotFound(err) {
				return "", f, fmt.Errorf("problem creating pipeline, missing tekton?: %v", err)
			}
			return "", f, fmt.Errorf("problem creating pipeline: %v", err)
		}
	}

	registry, err := docker.GetRegistry(image)
	if err != nil {
		return "", f, fmt.Errorf("problem in resolving image registry name: %v", err)
	}

	creds, err := pp.credentialsProvider(ctx, image)
	if err != nil {
		return "", f, err
	}

	// TODO(lkingland):  This registry defaulting logic
	// is either incorrect or in the wrong place.  At this stage of the
	// process registry should already be defined/defaulted, and this
	// function should be creating resources and deploying.   Missing
	// data (like registry) should have failed early in the process
	if registry == name.DefaultRegistry {
		registry = authn.DefaultAuthKey
	}
	if f.Registry == "" {
		f.Registry = registry
	}

	err = k8s.EnsureDockerRegistrySecretExist(ctx, getPipelineSecretName(f), namespace, labels, f.Deploy.Annotations, creds.Username, creds.Password, registry)
	if err != nil {
		return "", f, fmt.Errorf("problem in creating secret: %v", err)
	}

	err = createAndApplyPipelineRunTemplate(f, namespace, labels)
	if err != nil {
		return "", f, fmt.Errorf("problem in creating pipeline run: %v", err)
	}

	// we need to give k8s time to actually create the Pipeline Run
	time.Sleep(1 * time.Second)

	newestPipelineRun, err := findNewestPipelineRunWithRetry(ctx, f, namespace, client)
	if err != nil {
		return "", f, fmt.Errorf("problem in listing pipeline runs: %v", err)
	}

	err = pp.watchPipelineRunProgress(ctx, newestPipelineRun, namespace)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			return "", f, fmt.Errorf("problem in watching started pipeline run: %v", err)
		}
		// TODO replace deletion with pipeline-run cancellation
		_ = client.PipelineRuns(namespace).Delete(context.TODO(), newestPipelineRun.Name, metav1.DeleteOptions{})
		return "", f, fmt.Errorf("pipeline run cancelled: %w", context.Canceled)
	}

	newestPipelineRun, err = client.PipelineRuns(namespace).Get(ctx, newestPipelineRun.Name, metav1.GetOptions{})
	if err != nil {
		return "", f, fmt.Errorf("problem in retriving pipeline run status: %v", err)
	}

	if newestPipelineRun.Status.GetCondition(apis.ConditionSucceeded).Status == corev1.ConditionFalse {
		message := getFailedPipelineRunLog(ctx, client, newestPipelineRun, namespace)
		return "", f, fmt.Errorf("function pipeline run has failed with message: \n\n%s", message)
	}

	kClient, err := knative.NewServingClient(namespace)
	if err != nil {
		return "", f, fmt.Errorf("problem in retrieving status of deployed function: %v", err)
	}

	ksvc, err := kClient.GetService(ctx, f.Name)
	if err != nil {
		return "", f, fmt.Errorf("problem in retrieving status of deployed function: %v", err)
	}

	if ksvc.Generation == 1 {
		fmt.Fprintf(os.Stderr, "✅ Function deployed in namespace %q and exposed at URL: \n   %s\n", ksvc.Namespace, ksvc.Status.URL.String())
	} else {
		fmt.Fprintf(os.Stderr, "✅ Function updated in namespace %q and exposed at URL: \n   %s\n", ksvc.Namespace, ksvc.Status.URL.String())
	}

	if ksvc.Namespace != namespace {
		fmt.Fprintf(os.Stderr, "Warning: Final ksvc namespace %q does not match expected %q", ksvc.Namespace, namespace)
	}

	return ksvc.Status.URL.String(), f, nil
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

	const nobodyID = 65534

	const up = ".." + string(os.PathSeparator)
	go func() {
		tw := tar.NewWriter(pw)

		err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir,
			Name:     "source/",
			Mode:     0777,
			Uid:      nobodyID,
			Gid:      nobodyID,
			Uname:    "nobody",
			Gname:    "nobody",
		})
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("error while creating tar stream from sources: %w", err))
		}

		err = filepath.Walk(f.Root, func(p string, fi fs.FileInfo, err error) error {
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
					if strings.HasPrefix(lnk, up) || lnk == ".." {
						return fmt.Errorf("link %q points outside source root", p)
					}
				} else {
					t, err := filepath.Rel(f.Root, filepath.Join(filepath.Dir(p), lnk))
					if err != nil {
						return fmt.Errorf("cannot get relative path for symlink: %w", err)
					}
					if strings.HasPrefix(t, up) || t == ".." {
						return fmt.Errorf("link %q points outside source root", p)
					}
				}
			}

			hdr, err := tar.FileInfoHeader(fi, filepath.ToSlash(lnk))
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

// Remove tries to remove all resources that are present on the cluster and belongs to the input function and it's pipelines
func (pp *PipelinesProvider) Remove(ctx context.Context, f fn.Function) error {
	return pp.removeClusterResources(ctx, f)
}

// removeClusterResources tries to remove all resources that are present on the cluster and belongs to the input function and it's pipelines
// if there are any errors during the removal, string with error messages is returned
// if there are no error the returned string is empty
func (pp *PipelinesProvider) removeClusterResources(ctx context.Context, f fn.Function) error {
	// expect deployed namespace to be defined since trying to delete
	// a function (and its resources)
	if f.Deploy.Namespace == "" {
		fmt.Print("no namespace defined when trying to delete all resources on cluster regarding function and its pipelines\n")
		return fn.ErrNamespaceRequired
	}
	namespace := f.Deploy.Namespace

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
		deletePACRepositories,
	}

	wg.Add(len(deleteFunctions))
	errChan := make(chan error, len(deleteFunctions))

	for i := range deleteFunctions {
		df := deleteFunctions[i]
		go func() {
			defer wg.Done()
			err := df(ctx, namespace, listOptions)
			if err != nil && !k8serrors.IsNotFound(err) && !k8serrors.IsForbidden(err) {
				errChan <- err
			}
		}()
	}
	wg.Wait()
	close(errChan)

	// collect all errors and return them as a string
	errMsg := ""
	anyError := false
	for e := range errChan {
		if !anyError {
			anyError = true
			errMsg = "error deleting resources:"
		}
		errMsg += fmt.Sprintf("\n %v", e)
	}
	if errMsg != "" {
		return errors.New(errMsg)
	}
	return nil
}

// watchPipelineRunProgress watches the progress of the input PipelineRun
// and prints detailed description of the currently executed Tekton Task.
func (pp *PipelinesProvider) watchPipelineRunProgress(ctx context.Context, pr *v1.PipelineRun, namespace string) error {
	taskProgressMsg := map[string]string{
		"fetch-sources": "Fetching git repository with the function source code",
		"build":         "Building function image on the cluster",
		"deploy":        "Deploying function to the cluster",
	}

	clients, err := NewTektonClients()
	if err != nil {
		return err
	}

	prTracker := pipelinerun.NewTracker(pr.Name, namespace, clients)
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
				fmt.Fprintf(os.Stderr, "Running Pipeline Task: %s\n", taskDescription)

			}(run)
		}
	}
	wg.Wait()

	return err
}

// getFailedPipelineRunLog returns log message for a failed PipelineRun,
// returns log from a container where the failing TaskRun is running, if available.
func getFailedPipelineRunLog(ctx context.Context, client *pipelineClient.TektonV1Client, pr *v1.PipelineRun, namespace string) string {
	// Reason "Failed" usually means there is a specific failure in some step,
	// let's find the failed step and try to get log directly from the container.
	// If we are not able to get the container's log, we return the generic message from the PipelineRun.Status.
	message := pr.Status.GetCondition(apis.ConditionSucceeded).Message
	if pr.Status.GetCondition(apis.ConditionSucceeded).Reason == "Failed" {
		for _, ref := range pr.Status.ChildReferences {
			t, err := client.TaskRuns(namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Sprintf("error getting TaskRun %s: %v", ref.Name, err)
			}
			if t.Status.GetCondition(apis.ConditionSucceeded).Status == corev1.ConditionFalse {
				for _, s := range t.Status.Steps {
					// let's try to print logs of the first unsuccessful step
					if s.Terminated != nil && s.Terminated.ExitCode != 0 {
						podLogs, err := k8s.GetPodLogs(ctx, namespace, t.Status.PodName, s.Container)
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

// findNewestPipelineRunWithRetry tries to find newest Pipeline Run for the input function
func findNewestPipelineRunWithRetry(ctx context.Context, f fn.Function, namespace string, client *pipelineClient.TektonV1Client) (*v1.PipelineRun, error) {
	l := k8slabels.SelectorFromSet(k8slabels.Set(map[string]string{fnlabels.FunctionNameKey: f.Name}))
	listOptions := metav1.ListOptions{
		LabelSelector: l.String(),
	}

	var newestPipelineRun *v1.PipelineRun
	for attempt := 1; attempt <= 3; attempt++ {
		prs, err := client.PipelineRuns(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, fmt.Errorf("problem in listing pipeline runs: %v", err)
		}

		for _, pr := range prs.Items {
			currentPipelineRun := pr
			if len(prs.Items) < 1 || currentPipelineRun.Status.StartTime == nil {
				// Restart if StartTime is nil
				break
			}

			if newestPipelineRun == nil || currentPipelineRun.Status.StartTime.After(newestPipelineRun.Status.StartTime.Time) {
				newestPipelineRun = &currentPipelineRun
			}
		}

		// If a non-nil newestPipelineRun is found, break the retry loop
		if newestPipelineRun != nil {
			return newestPipelineRun, nil
		}
	}

	return nil, fmt.Errorf("problem in listing pipeline runs: haven't found any")
}

// allows simple mocking in unit tests, use with caution regarding concurrency
var createPersistentVolumeClaim = k8s.CreatePersistentVolumeClaim

func createPipelinePersistentVolumeClaim(ctx context.Context, f fn.Function, namespace string, labels map[string]string) error {
	var err error
	pvcs := DefaultPersistentVolumeClaimSize
	if f.Build.PVCSize != "" {
		if pvcs, err = resource.ParseQuantity(f.Build.PVCSize); err != nil {
			return fmt.Errorf("PVC size value could not be parsed. %w", err)
		}
	}
	err = createPersistentVolumeClaim(ctx, getPipelinePvcName(f), namespace, labels, f.Deploy.Annotations, corev1.ReadWriteOnce, pvcs, f.Build.RemoteStorageClass)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("problem creating persistent volume claim: %v", err)
	}
	return nil
}
