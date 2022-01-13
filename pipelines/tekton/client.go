package tekton

import (
	"fmt"
	"time"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"

	"knative.dev/kn-plugin-func/k8s"
)

const (
	DefaultWaitingTimeout = 120 * time.Second
)

func NewTektonClient() (*v1beta1.TektonV1beta1Client, error) {
	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton client: %v", err)
	}

	client, err := v1beta1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton client: %v", err)
	}

	return client, nil
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
