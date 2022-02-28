package tekton

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/tektoncd/cli/pkg/pipelinerun"
	"github.com/tektoncd/cli/pkg/taskrun"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/k8s"
	"knative.dev/kn-plugin-func/k8s/labels"
	"knative.dev/kn-plugin-func/knative"
	"knative.dev/pkg/apis"
)

type Opt func(*PipelinesProvider)

type PipelinesProvider struct {
	// namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, pipeline creation/run will commence to the configured namespace.
	namespace           string
	verbose             bool
	progressListener    fn.ProgressListener
	credentialsProvider docker.CredentialsProvider
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

func NewPipelinesProvider(opts ...Opt) *PipelinesProvider {
	pp := &PipelinesProvider{}

	for _, opt := range opts {
		opt(pp)
	}

	return pp
}

// Run creates a Tekton Pipeline and all necessary resources (PVCs, Secrets, SAs,...) for the input Function.
// It ensures that all needed resources are present on the cluster so the PipelineRun can be initialized.
// After the PipelineRun is being intitialized, the progress of the PipelineRun is being watched and printed to the output.
func (pp *PipelinesProvider) Run(ctx context.Context, f fn.Function) error {
	var err error

	if pp.namespace == "" {
		pp.namespace, err = k8s.GetNamespace(pp.namespace)
		if err != nil {
			return err
		}
	}

	pp.progressListener.Increment("Creating Pipeline resources")

	client, err := NewTektonClient()
	if err != nil {
		return err
	}

	// let's specify labels that will be applied to every resouce that is created for a Pipeline
	labels := map[string]string{labels.FunctionNameKey: f.Name}

	err = k8s.CreatePersistentVolumeClaim(ctx, getPipelinePvcName(f), pp.namespace, labels, corev1.ReadWriteOnce, *resource.NewQuantity(DefaultPersistentVolumeClaimSize, resource.DecimalSI))
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem creating persistent volume claim: %v", err)
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
		return err
	}

	_, err = k8s.GetSecret(ctx, getPipelineSecretName(f), pp.namespace)
	if errors.IsNotFound(err) {
		pp.progressListener.Stopping()
		creds, err := pp.credentialsProvider(ctx, registry)
		if err != nil {
			return err
		}
		pp.progressListener.Increment("Creating Pipeline resources")

		if registry == name.DefaultRegistry {
			registry = authn.DefaultAuthKey
		}

		err = k8s.CreateDockerRegistrySecret(ctx, getPipelineSecretName(f), pp.namespace, labels, creds.Username, creds.Password, registry)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("problem in creating secret: %v", err)
	}

	err = k8s.CreateServiceAccountWithSecret(ctx, getPipelineBuilderServiceAccountName(f), pp.namespace, labels, getPipelineSecretName(f))
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem in creating service account: %v", err)
		}
	}

	// using ClusterRole `knative-serving-namespaced-admin` that should be present on the cluster after the installation of Knative Serving
	err = k8s.CreateRoleBindingForServiceAccount(ctx, getPipelineDeployerRoleBindingName(f), pp.namespace, labels, getPipelineBuilderServiceAccountName(f), "ClusterRole", "knative-serving-namespaced-admin")
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem in creating role biding: %v", err)
		}
	}

	pp.progressListener.Increment("Running Pipeline with the Function")
	pr, err := client.PipelineRuns(pp.namespace).Create(ctx, generatePipelineRun(f, labels), metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("problem in creating pipeline run: %v", err)
	}

	err = pp.watchPipelineRunProgress(pr)
	if err != nil {
		return fmt.Errorf("problem in watching started pipeline run: %v", err)
	}

	pr, err = client.PipelineRuns(pp.namespace).Get(ctx, pr.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("problem in retriving pipeline run status: %v", err)
	}

	if pr.Status.GetCondition(apis.ConditionSucceeded).Status == corev1.ConditionFalse {
		return fmt.Errorf("function pipeline run has failed, please inspect logs of Tekton PipelineRun \"%s\"", pr.Name)
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
		pp.progressListener.Increment(fmt.Sprintf("Function deployed at URL: %s", ksvc.Status.URL.String()))
	} else {
		pp.progressListener.Increment(fmt.Sprintf("Function updated at URL: %s", ksvc.Status.URL.String()))
	}

	return nil
}

func (pp *PipelinesProvider) Remove(ctx context.Context, f fn.Function) error {

	l := k8slabels.SelectorFromSet(k8slabels.Set(map[string]string{labels.FunctionNameKey: f.Name}))
	listOptions := metav1.ListOptions{
		LabelSelector: l.String(),
	}

	// let's try to delete all resources in parallel, so the operation doesn't take long
	wg := sync.WaitGroup{}
	deleteFunctions := []func(context.Context, string, metav1.ListOptions) error{
		deletePipelines,
		deletePipelineRuns,
		k8s.DeleteRoleBindings,
		k8s.DeleteServiceAccounts,
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
			if err != nil && !errors.IsNotFound(err) {
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
func (pp *PipelinesProvider) watchPipelineRunProgress(pr *v1beta1.PipelineRun) error {
	taskProgressMsg := map[string]string{
		"fetch-repository": "Fetching git repository with the function source code",
		"build":            "Building function image on the cluster",
		"image-digest":     "Retrieving digest of the produced function image",
		"deploy":           "Deploying function to the cluster",
	}

	clientset, err := NewTektonClientset()
	if err != nil {
		return err
	}

	prTracker := pipelinerun.NewTracker(pr.Name, pp.namespace, clientset)
	trChannel := prTracker.Monitor([]string{})

	wg := sync.WaitGroup{}
	for trs := range trChannel {
		wg.Add(len(trs))

		for _, run := range trs {
			go func(tr taskrun.Run) {
				defer wg.Done()

				// let's print a Task name, if we don't have a proper description of the Task
				taskDescription := tr.Task
				if val, ok := taskProgressMsg[tr.Task]; ok {
					taskDescription = val
				}
				pp.progressListener.Increment(fmt.Sprintf("Running Pipeline: %s", taskDescription))

			}(run)
		}
	}
	wg.Wait()

	return nil
}
