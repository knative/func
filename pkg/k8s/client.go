package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

func NewClientAndResolvedNamespace(defaultNamespace string) (client *kubernetes.Clientset, namespace string, err error) {
	namespace, err = GetNamespace(defaultNamespace)
	if err != nil {
		return
	}

	client, err = NewKubernetesClientset()
	return
}

func NewKubernetesClientset() (*kubernetes.Clientset, error) {
	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}

	return kubernetes.NewForConfig(restConfig)
}

func GetNamespace(defaultNamespace string) (namespace string, err error) {
	namespace = defaultNamespace

	if defaultNamespace == "" {
		namespace, _, err = GetClientConfig().Namespace()
		if err != nil {
			return
		}
	}
	return
}

func GetClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
}
