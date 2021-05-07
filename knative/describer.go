package knative

import (
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/eventing/pkg/apis/eventing/v1beta1"

	fn "github.com/boson-project/func"
)

type Describer struct {
	Verbose   bool
	namespace string
}

func NewDescriber(namespaceOverride string) (describer *Describer, err error) {
	describer = &Describer{}
	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}

	describer.namespace = namespace
	return
}

// Describe by name. Note that the consuming API uses domain style notation, whereas Kubernetes
// restricts to label-syntax, which is thus escaped. Therefore as a knative (kube) implementation
// detal proper full names have to be escaped on the way in and unescaped on the way out. ex:
// www.example-site.com -> www-example--site-com
func (d *Describer) Describe(name string) (description fn.Description, err error) {

	servingClient, err := NewServingClient(d.namespace)
	if err != nil {
		return
	}

	eventingClient, err := NewEventingClient(d.namespace)
	if err != nil {
		return
	}

	service, err := servingClient.GetService(name)
	if err != nil {
		return
	}

	routes, err := servingClient.ListRoutes(v1.WithService(name))
	if err != nil {
		return
	}

	routeURLs := make([]string, 0, len(routes.Items))
	for _, route := range routes.Items {
		routeURLs = append(routeURLs, route.Status.URL.String())
	}

	triggers, err := eventingClient.ListTriggers()
	// IsNotFound -- Eventing is probably not installed on the cluster
	if err != nil && !errors.IsNotFound(err) {
		return
	}

	triggerMatches := func(t *v1beta1.Trigger) bool {
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

	description.Name = name
	description.Namespace = d.namespace
	description.Routes = routeURLs
	description.Subscriptions = subscriptions

	return
}
