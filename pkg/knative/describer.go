package knative

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	clientservingv1 "knative.dev/client-pkg/pkg/serving/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Describer struct {
	namespace string
	verbose   bool
}

func NewDescriber(namespaceOverride string, verbose bool) *Describer {
	return &Describer{
		namespace: namespaceOverride,
		verbose:   verbose,
	}
}

// Describe by name. Note that the consuming API uses domain style notation, whereas Kubernetes
// restricts to label-syntax, which is thus escaped. Therefore as a knative (kube) implementation
// detal proper full names have to be escaped on the way in and unescaped on the way out. ex:
// www.example-site.com -> www-example--site-com
func (d *Describer) Describe(ctx context.Context, name string) (description fn.Instance, err error) {
	if d.namespace == "" {
		d.namespace, err = k8s.GetDefaultNamespace()
		if err != nil {
			return fn.Instance{}, err
		}
	}

	servingClient, err := NewServingClient(d.namespace)
	if err != nil {
		return
	}

	eventingClient, err := NewEventingClient(d.namespace)
	if err != nil {
		return
	}

	service, err := servingClient.GetService(ctx, name)
	if err != nil {
		return
	}

	routes, err := servingClient.ListRoutes(ctx, clientservingv1.WithService(name))
	if err != nil {
		return
	}

	routeURLs := make([]string, 0, len(routes.Items))
	for _, route := range routes.Items {
		routeURLs = append(routeURLs, route.Status.URL.String())
	}

	primaryRouteURL := ""
	if len(routes.Items) > 0 {
		primaryRouteURL = routes.Items[0].Status.URL.String()
	}

	description.Name = name
	description.Namespace = d.namespace
	description.Route = primaryRouteURL
	description.Routes = routeURLs

	triggers, err := eventingClient.ListTriggers(ctx)
	// IsNotFound -- Eventing is probably not installed on the cluster
	if err != nil && !errors.IsNotFound(err) {
		err = nil
		return
	} else if err != nil {
		return
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

	return
}
