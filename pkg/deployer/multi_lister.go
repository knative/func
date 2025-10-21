package deployer

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Getter interface {
	Get(ctx context.Context, name, namespace string) (fn.ListItem, error)
}

type Lister struct {
	verbose bool

	knativeGetter    Getter
	kubernetesGetter Getter
}

func NewLister(verbose bool, knativeGetter, kubernetesGetter Getter) fn.Lister {
	return &Lister{
		verbose:          verbose,
		knativeGetter:    knativeGetter,
		kubernetesGetter: kubernetesGetter,
	}
}

func (d *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client: %v", err)
	}

	serviceClient := clientset.CoreV1().Services(namespace)

	services, err := serviceClient.List(ctx, metav1.ListOptions{
		LabelSelector: "function.knative.dev/name",
	})
	if err != nil {
		return nil, fmt.Errorf("unable to list services: %v", err)
	}

	listItems := make([]fn.ListItem, 0, len(services.Items))
	for _, service := range services.Items {
		if _, ok := service.Labels["serving.knative.dev/revision"]; ok {
			// skip the services for Knative Serving revisions, as we only take care on the "parent" ones
			continue
		}

		deployType, ok := service.Annotations[DeployTypeAnnotation]
		if !ok {
			// fall back to the Knative Describer in case no annotation is given
			item, err := d.knativeGetter.Get(ctx, service.Name, namespace)
			if err != nil {
				return nil, fmt.Errorf("unable to get details about function: %v", err)
			}

			listItems = append(listItems, item)
			continue
		}

		var item fn.ListItem
		switch deployType {
		case KnativeDeployerName:
			item, err = d.knativeGetter.Get(ctx, service.Name, namespace)
		case KubernetesDeployerName:
			item, err = d.kubernetesGetter.Get(ctx, service.Name, namespace)
		default:
			return nil, fmt.Errorf("unknown deploy type %s for function %s/%s", deployType, service.Name, service.Namespace)
		}

		if err != nil {
			return nil, fmt.Errorf("unable to get details about function: %v", err)
		}

		listItems = append(listItems, item)
	}

	return listItems, nil
}
