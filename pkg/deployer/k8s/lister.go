package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Lister struct {
	verbose bool
}

func NewLister(verbose bool) *Lister {
	return &Lister{verbose: verbose}
}

// List functions, optionally specifying a namespace.
func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)
	deployments, err := deploymentClient.List(ctx, metav1.ListOptions{
		LabelSelector: "function.knative.dev/name",
	})
	if err != nil {
		return nil, fmt.Errorf("could not list deployments: %w", err)
	}

	items := []fn.ListItem{}
	for _, deployment := range deployments.Items {

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range deployment.Status.Conditions {
			if con.Type == v1.DeploymentAvailable {
				ready = con.Status
				break
			}
		}

		service, err := serviceClient.Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("could not get service: %w", err)
		}

		runtimeLabel := ""
		listItem := fn.ListItem{
			Name:      service.Name,
			Namespace: service.Namespace,
			Runtime:   runtimeLabel,
			URL:       fmt.Sprintf("http://%s.%s.svc", service.Name, service.Namespace), // TODO: use correct scheme
			Ready:     string(ready),
		}

		items = append(items, listItem)
	}

	return items, nil
}
