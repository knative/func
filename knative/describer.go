package knative

import (
	"fmt"
	"github.com/boson-project/faas/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/eventing/pkg/apis/eventing/v1alpha1"
	eventingv1client "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1alpha1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
)

type Describer struct {
	Verbose        bool
	namespace      string
	servingClient  *servingv1client.ServingV1alpha1Client
	eventingClient *eventingv1client.EventingV1alpha1Client
	config 		   *rest.Config
}

func NewDescriber(namespace string) (describer *Describer, err error) {
	describer = &Describer{namespace: namespace}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	if describer.namespace == "" {
		namespace, _, err := clientConfig.Namespace()
		if err == nil {
			describer.namespace = namespace
		}
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return
	}
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

func (describer *Describer) Describe(name string) (description client.FunctionDescription, err error) {

	namespace := describer.namespace
	servingClient := describer.servingClient
	eventingClient := describer.eventingClient

	service, err := servingClient.Services(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return
	}

	serviceLabel := fmt.Sprintf("serving.knative.dev/service=%s", name)
	routes, err := servingClient.Routes(namespace).List(metav1.ListOptions{LabelSelector: serviceLabel})
	if err != nil {
		return
	}

	routeURLs := make([]string, 0, len(routes.Items))
	for _, route := range routes.Items {
		routeURLs = append(routeURLs, route.Status.URL.String())
	}

	triggers, err := eventingClient.Triggers(namespace).List(metav1.ListOptions{})
	if err != nil {
		return
	}

	triggerMatches := func(t *v1alpha1.Trigger) bool {
		return (t.Spec.Subscriber.Ref != nil && t.Spec.Subscriber.Ref.Name == service.Name) ||
			(t.Spec.Subscriber.URI != nil && service.Status.Address != nil && service.Status.Address.URL != nil &&
				t.Spec.Subscriber.URI.Path == service.Status.Address.URL.Path)

	}

	subscriptions := make([]client.Subscription, 0, len(triggers.Items))
	for _, trigger := range triggers.Items {
		if triggerMatches(&trigger) {
			filterAttrs := *trigger.Spec.Filter.Attributes
			subscription := client.Subscription{
				Source: filterAttrs["source"],
				Type:   filterAttrs["type"],
				Broker: trigger.Spec.Broker,
			}
			subscriptions = append(subscriptions, subscription)
		}
	}

	description.Name = service.Name
	description.Routes = routeURLs
	description.Subscriptions = subscriptions

	return
}
