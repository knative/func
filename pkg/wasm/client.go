package wasm

import (
	"fmt"

	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"

	"knative.dev/func/pkg/k8s"
)

// ClientsetProvider creates a WasmModule clientset.
// It is injectable for testing.
type ClientsetProvider func() (wasmclientset.Interface, error)

// defaultClientsetProvider creates a real WasmModule clientset from
// the current kubeconfig.
func defaultClientsetProvider() (wasmclientset.Interface, error) {
	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientSetup, err)
	}

	cs, err := wasmclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientSetup, err)
	}

	return cs, nil
}
