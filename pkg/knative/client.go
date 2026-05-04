package knative

import (
	"fmt"
	"os"
	"path/filepath"

	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	eventingv1 "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1"
	servingv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewServingClient(namespace string) (clientservingv1.KnServingClient, error) {
	if err := validateKubeconfigFile(); err != nil {
		return nil, err
	}

	restConfig, err := k8s.GetClientConfig().ClientConfig()
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

func NewEventingClient(namespace string) (clienteventingv1.KnEventingClient, error) {
	if err := validateKubeconfigFile(); err != nil {
		return nil, err
	}

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new serving client: %v", err)
	}

	eventingClient, err := eventingv1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new eventing client: %v", err)
	}

	client := clienteventingv1.NewKnEventingClient(eventingClient, namespace)

	return client, nil
}

// validateKubeconfigFile checks if explicitly set KUBECONFIG path exists
func validateKubeconfigFile() error {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		return nil
	}

	for _, path := range filepath.SplitList(kubeconfigPath) {
		if path == "" {
			continue
		}
		_, statErr := os.Stat(path)
		if statErr == nil {
			return nil
		}
		if !os.IsNotExist(statErr) {
			return fmt.Errorf("%w: kubeconfig file is not accessible at path: %s", fn.ErrInvalidKubeconfig, path)
		}
	}

	return fmt.Errorf("%w: kubeconfig file does not exist at path: %s", fn.ErrInvalidKubeconfig, kubeconfigPath)
}
