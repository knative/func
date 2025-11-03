package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
)

type Describer struct {
	verbose bool
}

func NewDescriber(verbose bool) *Describer {
	return &Describer{
		verbose: verbose,
	}
}

// Describe a function by name. Note that the consuming API uses domain style
// notation, whereas Kubernetes restricts to label-syntax, which is thus
// escaped. Therefor as a knative (kube) implementation detail proper full
// names have to be escaped on the way in and unescaped on the way out. ex:
// www.example-site.com -> www-example--site-com
func (d *Describer) Describe(ctx context.Context, name, namespace string) (*fn.Instance, error) {
	if namespace == "" {
		return nil, fmt.Errorf("function namespace is required when describing %q", name)
	}

	clientset, err := NewKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client: %v", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	service, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get service for function: %v", err)
	}

	if !UsesRawDeployer(service.Annotations) {
		return nil, nil
	}

	description := &fn.Instance{
		Name:       name,
		Namespace:  namespace,
		DeployType: KubernetesDeployerName,
	}

	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return description, fmt.Errorf("unable to get deployment %q: %v", name, err)
	}

	primaryRouteURL := fmt.Sprintf("http://%s.%s.svc", name, namespace) // TODO: get correct scheme?
	description.Route = primaryRouteURL
	description.Routes = []string{primaryRouteURL}

	// Populate labels from the deployment
	if deployment.Labels != nil {
		description.Labels = deployment.Labels
	}

	return description, nil
}
