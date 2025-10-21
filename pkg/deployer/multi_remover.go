package deployer

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type MultiRemover struct {
	verbose bool

	knativeRemover    fn.Remover
	kubernetesRemover fn.Remover
}

func NewMultiRemover(verbose bool, knativeRemover, kubernetesRemover fn.Remover) *MultiRemover {
	return &MultiRemover{
		verbose:           verbose,
		knativeRemover:    knativeRemover,
		kubernetesRemover: kubernetesRemover,
	}
}

func (d *MultiRemover) Remove(ctx context.Context, name, namespace string) (err error) {
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fmt.Errorf("unable to create k8s client: %v", err)
	}

	serviceClient := clientset.CoreV1().Services(namespace)

	service, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get service for function: %v", err)
	}

	deployType, ok := service.Annotations[DeployTypeAnnotation]
	if !ok {
		// fall back to the Knative Remover in case no annotation is given
		return d.knativeRemover.Remove(ctx, name, namespace)
	}

	switch deployType {
	case KnativeDeployerName:
		return d.knativeRemover.Remove(ctx, name, namespace)
	case KubernetesDeployerName:
		return d.kubernetesRemover.Remove(ctx, name, namespace)
	default:
		return fmt.Errorf("unknown deploy type: %s", deployType)
	}
}
