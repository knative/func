package knative

import (
	"fmt"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	clienteventingv1beta1 "knative.dev/client/pkg/eventing/v1beta1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	eventingv1beta1 "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1beta1"
	servingv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
)

const (
	DefaultWaitingTimeout = 60 * time.Second
)

func NewServingClient(namespace string) (clientservingv1.KnServingClient, error) {

	restConfig, err := getClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	servingClient, err := servingv1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	client := clientservingv1.NewKnServingClient(servingClient, namespace)

	return client, nil
}

func NewEventingClient(namespace string) (clienteventingv1beta1.KnEventingClient, error) {

	restConfig, err := getClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	eventingClient, err := eventingv1beta1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new eventing client: %v", err)
	}

	client := clienteventingv1beta1.NewKnEventingClient(eventingClient, namespace)

	return client, nil
}

func GetNamespace(defaultNamespace string) (namespace string, err error) {
	namespace = defaultNamespace

	if defaultNamespace == "" {
		namespace, _, err = getClientConfig().Namespace()
		if err != nil {
			return
		}
	}
	return
}

func getClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
}
