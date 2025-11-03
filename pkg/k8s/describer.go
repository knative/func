package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
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

	eventingClient, err := NewEventingClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("unable to create eventing client: %v", err)
	}

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

	triggers, err := eventingClient.ListTriggers(ctx)
	// IsNotFound -- Eventing is probably not installed on the cluster
	if err != nil && !errors.IsNotFound(err) {
		return description, nil
	} else if err != nil {
		return description, fmt.Errorf("unable to list triggers: %v", err)
	}

	triggerMatches := func(t *eventingv1.Trigger) bool {
		return t.Spec.Subscriber.Ref != nil &&
			t.Spec.Subscriber.Ref.Name == name &&
			t.Spec.Subscriber.Ref.APIVersion == "v1" &&
			t.Spec.Subscriber.Ref.Kind == "Service"
	}

	subscriptions := make([]fn.Subscription, 0, len(triggers.Items))
	for _, trigger := range triggers.Items {
		if triggerMatches(&trigger) {
			filterAttrs := trigger.Spec.Filter.Attributes
			subscription := fn.Subscription{
				Source: filterAttrs["source"],
				Type:   filterAttrs["type"],
				Broker: trigger.Spec.Broker,
			}
			subscriptions = append(subscriptions, subscription)
		}
	}

	description.Subscriptions = subscriptions

	// Populate labels from the deployment
	if deployment.Labels != nil {
		description.Labels = deployment.Labels
	}

	return description, nil
}
