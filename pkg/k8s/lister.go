package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s/labels"
)

type Lister struct {
	kc      *Client
	verbose bool
}

func NewLister(kc *Client, verbose bool) fn.Lister {
	return &Lister{
		kc:      kc,
		verbose: verbose,
	}
}

func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	if l.kc == nil {
		return nil, fmt.Errorf("kubernetes client is not initialized")
	}
	clientset, err := l.kc.Clientset()
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

		item, err := l.get(ctx, clientset, service.Name, service.Namespace)
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

	// External hostname (if exposed) was recorded on the Service by Deploy()
	// at exposure time - no extra API call is needed here.
	url := fmt.Sprintf("http://%s.%s.svc", service.Name, service.Namespace) // TODO: use correct scheme
	if hostname, ok := service.Annotations[RouteHostnameAnnotation]; ok && hostname != "" {
		url = fmt.Sprintf("https://%s", hostname)
	}

	listItem := fn.ListItem{
		Name:      service.Name,
		Namespace: service.Namespace,
		Runtime:   deployment.Labels[labels.FunctionRuntimeKey],
		URL:       url,
		Ready:     string(ready),
		Deployer:  KubernetesDeployerName,
		Replicas:  int(deployment.Status.ReadyReplicas),
	}

	return listItem, nil
}
