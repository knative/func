package tekton

import (
	"fmt"
	"time"

	"github.com/tektoncd/cli/pkg/cli"
	v1 "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1"
	"knative.dev/func/pkg/k8s"
)

const (
	DefaultWaitingTimeout = 120 * time.Second
)

// NewTektonClient returns TektonV1beta1Client for namespace
func NewTektonClient(namespace string) (*v1.TektonV1Client, error) {
	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton client: %w", err)
	}

	client, err := v1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new tekton client: %v", err)
	}

	return client, nil
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
