package knative

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"knative.dev/eventing/pkg/apis/eventing/v1beta1"
	eventingv1client "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1beta1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
)

type Describer struct {
	Verbose        bool
	namespace      string
	servingClient  *servingv1client.ServingV1alpha1Client
	eventingClient *eventingv1client.EventingV1beta1Client
	config         *rest.Config
}

func NewDescriber(namespaceOverride string) (describer *Describer, err error) {
	describer = &Describer{}
	config, namespace, err := newClientConfig(namespaceOverride)
	if err != nil {
		return
	}
	describer.namespace = namespace

	describer.servingClient, err = servingv1client.NewForConfig(config)
	if err != nil {
		return
	}
	describer.eventingClient, err = eventingv1client.NewForConfig(config)
	if err != nil {
		return
	}
	describer.config = config
	return
}

// Describe by name. Note that the consuming API uses domain style notation, whereas Kubernetes
// restricts to label-syntax, which is thus escaped. Therefore as a knative (kube) implementation
// detal proper full names have to be escaped on the way in and unescaped on the way out. ex:
// www.example-site.com -> www-example--site-com
func (describer *Describer) Describe(name string) (description faas.Description, err error) {

	namespace := describer.namespace
	servingClient := describer.servingClient
	eventingClient := describer.eventingClient

	serviceName, err := k8s.ToK8sAllowedName(name)
	if err != nil {
		return
	}

	service, err := servingClient.Services(namespace).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	serviceLabel := fmt.Sprintf("serving.knative.dev/service=%s", serviceName)
	routes, err := servingClient.Routes(namespace).List(metav1.ListOptions{LabelSelector: serviceLabel})
	if err != nil {
		return
	}

	routeURLs := make([]string, 0, len(routes.Items))
	for _, route := range routes.Items {
		routeURLs = append(routeURLs, route.Status.URL.String())
	}

	triggers, err := eventingClient.Triggers(namespace).List(metav1.ListOptions{})
	// IsNotFound -- Eventing is probably not installed on the cluster
	if err != nil && !errors.IsNotFound(err) {
		return
	}

	triggerMatches := func(t *v1beta1.Trigger) bool {
		return (t.Spec.Subscriber.Ref != nil && t.Spec.Subscriber.Ref.Name == service.Name) ||
			(t.Spec.Subscriber.URI != nil && service.Status.Address != nil && service.Status.Address.URL != nil &&
				t.Spec.Subscriber.URI.Path == service.Status.Address.URL.Path)

	}

	subscriptions := make([]faas.Subscription, 0, len(triggers.Items))
	for _, trigger := range triggers.Items {
		if triggerMatches(&trigger) {
			filterAttrs := trigger.Spec.Filter.Attributes
			subscription := faas.Subscription{
				Source: filterAttrs["source"],
				Type:   filterAttrs["type"],
				Broker: trigger.Spec.Broker,
			}
			subscriptions = append(subscriptions, subscription)
		}
	}

	description.Routes = routeURLs
	description.Subscriptions = subscriptions
	description.Name, err = k8s.FromK8sAllowedName(service.Name)

	return
}
