package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	fn "knative.dev/func/pkg/functions"
)

type Lister struct {
	verbose bool
}

func NewLister(verbose bool) fn.Lister {
	return &Lister{
		verbose: verbose,
	}
}

func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	clientset, err := NewKubernetesClientset()
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
		if !UsesRawDeployer(service.Annotations) {
			continue
		}

		item, err := l.get(ctx, clientset, service.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("unable to get details about function: %v", err)
		}

		listItems = append(listItems, item)
	}

	return listItems, nil
}

// Get a function, optionally specifying a namespace.
func (l *Lister) get(ctx context.Context, clientset *kubernetes.Clientset, name, namespace string) (fn.ListItem, error) {
	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("could not get deployment: %w", err)
	}

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
		return fn.ListItem{}, fmt.Errorf("could not get service: %w", err)
	}

	runtimeLabel := ""
	listItem := fn.ListItem{
		Name:      service.Name,
		Namespace: service.Namespace,
		Runtime:   runtimeLabel,
		URL:       fmt.Sprintf("http://%s.%s.svc", service.Name, service.Namespace), // TODO: use correct scheme
		Ready:     string(ready),
		Deployer:  KubernetesDeployerName,
	}

	return listItem, nil
}
