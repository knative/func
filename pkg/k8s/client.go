package k8s

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	eventingv1 "knative.dev/client/pkg/eventing/v1"
	servingv1 "knative.dev/client/pkg/serving/v1"
	eventingclientsetv1 "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1"
	servingclientsetv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
)

func NewClientAndResolvedNamespace(ns string) (*kubernetes.Clientset, string, error) {
	var err error
	if ns == "" {
		ns, err = GetDefaultNamespace()
		if err != nil {
			return nil, ns, err
		}
	}

	client, err := NewKubernetesClientset()
	return client, ns, err
}

func NewKubernetesClientset() (*kubernetes.Clientset, error) {
	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}

	return kubernetes.NewForConfig(restConfig)
}

func NewDynamicClient() (dynamic.Interface, error) {
	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}

	return dynamic.NewForConfig(restConfig)
}

// TODO: same as for NewEventingClient
func NewServingClient(namespace string) (servingv1.KnServingClient, error) {

	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	servingClient, err := servingclientsetv1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	client := servingv1.NewKnServingClient(servingClient, namespace)

	return client, nil
}

// TODO: this should probably be moved to the knative package, but then we get cyclic dependencies
// (NewEventingClient uses GetClientConfig() from k8s, while the k8s deployer also needs NewEventingClient to create the triggers)
func NewEventingClient(namespace string) (eventingv1.KnEventingClient, error) {

	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	eventingClient, err := eventingclientsetv1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new eventing client: %v", err)
	}

	client := eventingv1.NewKnEventingClient(eventingClient, namespace)

	return client, nil
}

// GetDefaultNamespace returns default namespace
func GetDefaultNamespace() (namespace string, err error) {
	namespace, _, err = GetClientConfig().Namespace()
	return
}

func GetClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
}
