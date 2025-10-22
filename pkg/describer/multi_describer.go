package describer

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type MultiDescriber struct {
	verbose bool

	knativeDescriber    fn.Describer
	kubernetesDescriber fn.Describer
}

func NewMultiDescriber(verbose bool, knativeDescriber, kubernetesDescriber fn.Describer) *MultiDescriber {
	return &MultiDescriber{
		verbose:             verbose,
		knativeDescriber:    knativeDescriber,
		kubernetesDescriber: kubernetesDescriber,
	}
}

// Describe a function by name
func (d *MultiDescriber) Describe(ctx context.Context, name, namespace string) (fn.Instance, error) {
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to create k8s client: %v", err)
	}

	serviceClient := clientset.CoreV1().Services(namespace)

	service, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to get service for function: %v", err)
	}

	deployType, ok := service.Annotations[deployer.DeployTypeAnnotation]
	if !ok {
		// fall back to the Knative Describer in case no annotation is given
		return d.knativeDescriber.Describe(ctx, name, namespace)
	}

	switch deployType {
	case deployer.KnativeDeployerName:
		return d.knativeDescriber.Describe(ctx, name, namespace)
	case deployer.KubernetesDeployerName:
		return d.kubernetesDescriber.Describe(ctx, name, namespace)
	default:
		return fn.Instance{}, fmt.Errorf("unknown deploy type: %s", deployType)
	}
}
