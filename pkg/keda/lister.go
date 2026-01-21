package keda

import (
	"context"
	"fmt"

	"github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
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
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client: %v", err)
	}

	httpScaledObjectClientset, err := NewHTTPScaledObjectClientset()
	if err != nil {
		return nil, fmt.Errorf("unable to create HTTPScaledObject client: %v", err)
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
		if !UsesKedaDeployer(service.Annotations) {
			continue
		}

		item, err := l.get(ctx, httpScaledObjectClientset, service.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("unable to get details about function: %v", err)
		}

		listItems = append(listItems, item)
	}

	return listItems, nil
}

// Get a function, optionally specifying a namespace.
func (l *Lister) get(ctx context.Context, httpScaledObjectClientset *versioned.Clientset, name, namespace string) (fn.ListItem, error) {
	httpScaledObject, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("unable to get HTTPScaledObject: %v", err)
	}

	ready := v1.ConditionUnknown
	// HTTPScaledObject may have multiple Ready conditions, we need to find one that is True
	// TODO: use Status.Conditions.GetReadyCondition() as soon as this is fixed in http-add-on
	for _, condition := range httpScaledObject.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = v1.ConditionTrue
		}
	}

	url := ""
	if len(httpScaledObject.Spec.Hosts) > 0 {
		url = fmt.Sprintf("http://%s:8080", httpScaledObject.Spec.Hosts[0])
	}

	runtimeLabel := ""
	listItem := fn.ListItem{
		Name:      name,
		Namespace: namespace,
		Runtime:   runtimeLabel,
		URL:       url,
		Ready:     string(ready),
		Deployer:  KedaDeployerName,
	}

	return listItem, nil
}
