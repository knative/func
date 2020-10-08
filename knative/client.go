package knative

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	clienteventingv1beta1 "knative.dev/client/pkg/eventing/v1beta1"
	"knative.dev/client/pkg/kn/commands"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
)

const (
	DefaultWaitingTimeout = 60 * time.Second
)

func NewServingClient(namespace string, verbose bool) (clientservingv1.KnServingClient, io.Writer, error) {

	knParams := initKnParams(verbose)

	client, err := knParams.NewServingClient(namespace)
	if err != nil {
		return nil, knParams.Output, fmt.Errorf("failed to create new serving client: %v", err)
	}

	return client, knParams.Output, nil
}

func NewEventingClient(namespace string, verbose bool) (clienteventingv1beta1.KnEventingClient, io.Writer, error) {

	knParams := initKnParams(verbose)

	client, err := knParams.NewEventingClient(namespace)
	if err != nil {
		return nil, knParams.Output, fmt.Errorf("failed to create new eventing client: %v", err)
	}

	return client, knParams.Output, nil
}

func GetNamespace(defaultNamespace string) (namespace string, err error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	namespace = defaultNamespace

	if defaultNamespace == "" {
		namespace, _, err = clientConfig.Namespace()
		if err != nil {
			return
		}
	}
	return
}

func initKnParams(verbose bool) commands.KnParams {
	p := commands.KnParams{}
	p.Initialize()

	// Capture output in a buffer if verbose is not enabled for output on error.
	if verbose {
		p.Output = os.Stdout
	} else {
		p.Output = &bytes.Buffer{}
	}

	return p
}
