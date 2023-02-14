package tekton

import (
	"fmt"
	"time"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"

	"knative.dev/func/pkg/k8s"
)

const (
	DefaultWaitingTimeout = 120 * time.Second
)

func NewTektonClientAndResolvedNamespace(defaultNamespace string) (*v1beta1.TektonV1beta1Client, string, error) {
	namespace, err := k8s.GetNamespace(defaultNamespace)
	if err != nil {
		return nil, "", err
	}

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new tekton client: %w", err)
	}

	client, err := v1beta1.NewForConfig(restConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new tekton client: %v", err)
	}

	return client, namespace, nil
}

func NewTektonClientset() (versioned.Interface, error) {
	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton clientset: %v", err)
	}

	clientset, err := versioned.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton clientset: %v", err)
	}

	return clientset, nil
}
