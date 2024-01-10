package tekton

import (
	"fmt"
	"time"

	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1beta1"
	"knative.dev/func/pkg/k8s"
)

const (
	DefaultWaitingTimeout = 120 * time.Second
)

// NewTektonClientAndResolvedNamespace returns TektonV1beta1Client,namespace,error
func NewTektonClientAndResolvedNamespace(namespace string) (*v1beta1.TektonV1beta1Client, string, error) {
	var err error
	if namespace == "" {
		namespace, err = k8s.GetDefaultNamespace()
		if err != nil {
			return nil, "", err
		}
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

func NewTektonClients() (*cli.Clients, error) {
	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton clientset: %v", err)
	}

	params := cli.TektonParams{}
	clients, err := params.Clients(restConfig)

	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton clients: %v", err)
	}

	return clients, nil
}
