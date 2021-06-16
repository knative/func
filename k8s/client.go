package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)


func NewKubernetesClientset(namespace string) (*kubernetes.Clientset, error) {

	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %v", err)
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
