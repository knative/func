package knative

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
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
// escaped. Therefore as a knative (kube) implementation detal proper full
// names have to be escaped on the way in and unescaped on the way out. ex:
// www.example-site.com -> www-example--site-com
func (d *Describer) Describe(ctx context.Context, name, namespace string) (fn.Instance, error) {
	if namespace == "" {
		return fn.Instance{}, fmt.Errorf("function namespace is required when describing %q", name)
	}

	servingClient, err := NewServingClient(namespace)
	if err != nil {
		return fn.Instance{}, err
	}

	eventingClient, err := NewEventingClient(namespace)
	if err != nil {
		return fn.Instance{}, err
	}

	service, err := servingClient.GetService(ctx, name)
	if err != nil {
		// If we can't get the service, check why
		if IsCRDNotFoundError(err) {
			// Knative Serving not installed - we don't handle this
			return fn.Instance{}, fn.ErrNotHandled
		}
		if errors.IsNotFound(err) {
			// Service doesn't exist as a Knative service - we don't handle this
			return fn.Instance{}, fn.ErrNotHandled
		}
		// Some other error (permissions, network, etc.) - this is a real error
		// We can't determine if we should handle it, so propagate it
		return fn.Instance{}, fmt.Errorf("failed to check if service uses Knative: %w", err)
	}

	// We got the service, now check if we should handle it
	if !UsesKnativeDeployer(service.Annotations) {
		// no need to handle this service
		return fn.Instance{}, fn.ErrNotHandled
	}

	// We're responsible, for this function --> proceed...

	routes, err := servingClient.ListRoutes(ctx, clientservingv1.WithService(name))
	if err != nil {
		return fn.Instance{}, err
	}

	routeURLs := make([]string, 0, len(routes.Items))
	for _, route := range routes.Items {
		routeURLs = append(routeURLs, route.Status.URL.String())
	}

	primaryRouteURL := ""
	if len(routes.Items) > 0 {
		primaryRouteURL = routes.Items[0].Status.URL.String()
	}

	description := fn.Instance{
		Name:      name,
		Namespace: namespace,
		Deployer:  KnativeDeployerName,
		Route:     primaryRouteURL,
		Routes:    routeURLs,
		Labels:    service.Labels,
	}

	// get used image (including the sha)
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to create k8s client: %v", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	deployments, err := deploymentClient.List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("serving.knative.dev/revision=%s", service.Status.LatestCreatedRevisionName),
	})
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to list deployments of service: %w", err)
	}

	if len(deployments.Items) == 0 {
		return fn.Instance{}, fmt.Errorf("no deployments found for service %s", service.Name)
	}

	for _, container := range deployments.Items[0].Spec.Template.Spec.Containers {
		if container.Name == "user-container" {
			description.Image = container.Image
		}
	}

	if description.Image != "" {
		v, err := fn.MiddlewareVersion(description.Image)
		if err == nil {
			// don't fail on errors
			description.Middleware = fn.Middleware{
				Version: v,
			}
		}
	}

	triggers, err := eventingClient.ListTriggers(ctx)
	if err != nil {
		if errors.IsNotFound(err) || IsCRDNotFoundError(err) {
			// No trigger found or Eventing is probably not installed on the cluster --> we're done here
			return description, nil
		}

		return fn.Instance{}, err
	}

	triggerMatches := func(t *eventingv1.Trigger) bool {
		return (t.Spec.Subscriber.Ref != nil && t.Spec.Subscriber.Ref.Name == service.Name) ||
			(t.Spec.Subscriber.URI != nil && service.Status.Address != nil && service.Status.Address.URL != nil &&
				t.Spec.Subscriber.URI.Path == service.Status.Address.URL.Path)

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

	return description, nil
}
