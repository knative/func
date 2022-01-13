package tekton

import (
	"context"
	"fmt"
	"sync"

	"github.com/tektoncd/cli/pkg/pipelinerun"
	"github.com/tektoncd/cli/pkg/taskrun"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/k8s"
	"knative.dev/kn-plugin-func/knative"
	"knative.dev/kn-plugin-func/pipelines"
	"knative.dev/pkg/apis"
)

type Opt func(*PipelinesProvider) error

type PipelinesProvider struct {
	// namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, pipeline creation/run will commence to the configured namespace.
	namespace                            string
	Verbose                              bool
	progressListener                     fn.ProgressListener
	containerRegistryCredentialsCallback pipelines.ContainerRegistryCredentialsCallback
}

func WithNamespace(namespace string) Opt {
	return func(pp *PipelinesProvider) error {
		namespace, err := k8s.GetNamespace(namespace)
		if err != nil {
			return err
		}
		pp.namespace = namespace
		return nil
	}
}

func WithProgressListener(pl fn.ProgressListener) Opt {
	return func(pp *PipelinesProvider) error {
		pp.progressListener = pl
		return nil
	}
}

func WithPromptForContainerRegistryCredentials(cbk pipelines.ContainerRegistryCredentialsCallback) Opt {
	return func(pp *PipelinesProvider) error {
		pp.containerRegistryCredentialsCallback = cbk
		return nil
	}
}

func NewPipelinesProvider(opts ...Opt) (*PipelinesProvider, error) {
	pp := &PipelinesProvider{}

	for _, opt := range opts {
		err := opt(pp)
		if err != nil {
			return nil, err
		}
	}

	return pp, nil
}

// Run creates a Tekton Pipeline and all necessary resources (PVCs, Secrets, SAs,...) for the input Function.
// It ensures that all needed resources are present on the cluster so the PipelineRun can be initialized.
// After the PipelineRun is being intitialized, the progress of the PipelineRun is being watched and printed to the output.
func (pp *PipelinesProvider) Run(ctx context.Context, f fn.Function) error {
	pp.progressListener.Increment("Creating Pipeline resources")

	client, err := NewTektonClient()
	if err != nil {
		return err
	}

	err = k8s.CreatePersistentVolumeClaim(ctx, getPipelinePvcName(f), pp.namespace, corev1.ReadWriteOnce, *resource.NewQuantity(DefaultPersistentVolumeClaimSize, resource.DecimalSI))
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem creating persistent volume claim: %v", err)
		}
	}

	_, err = client.Pipelines(pp.namespace).Create(ctx, generatePipeline(f), metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			if errors.IsNotFound(err) {
				return fmt.Errorf("problem creating pipeline, missing tekton?: %v", err)
			}
			return fmt.Errorf("problem creating pipeline: %v", err)
		}
	}

	_, err = k8s.GetSecret(ctx, getPipelineSecretName(f), pp.namespace)
	if errors.IsNotFound(err) {
		pp.progressListener.Stopping()
		creds, err := pp.containerRegistryCredentialsCallback()
		if err != nil {
			return err
		}
		pp.progressListener.Increment("Creating Pipeline resources")

		err = k8s.CreateDockerRegistrySecret(ctx, getPipelineSecretName(f), pp.namespace, creds.Username, creds.Password, creds.Server)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("problem in creating secret: %v", err)
	}

	err = k8s.CreateServiceAccountWithSecret(ctx, getPipelineBuilderServiceAccountName(f), pp.namespace, getPipelineSecretName(f))
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem in creating service account: %v", err)
		}
	}

	err = k8s.CreateClusterRoleBindingForServiceAccount(ctx, getPipelineDeployerClusterRoleBindingName(f, pp.namespace), pp.namespace, getPipelineBuilderServiceAccountName(f), "knative-serving-namespaced-edit")
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("problem in creating cluster role biding: %v", err)
		}
	}

	pp.progressListener.Increment("Running Pipeline with the Function")
	pr, err := client.PipelineRuns(pp.namespace).Create(ctx, generatePipelineRun(f), metav1.CreateOptions{})
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
