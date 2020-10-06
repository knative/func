package knative

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/client/pkg/kn/commands"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
)

const (
	DefaultWaitingTimeout = 60 * time.Second
)

func NewClient(namespace string, verbose bool) (clientservingv1.KnServingClient, io.Writer, error) {

	p := commands.KnParams{}
	p.Initialize()

	// Capture output in a buffer if verbose is not enabled for output on error.
	if verbose {
		p.Output = os.Stdout
	} else {
		p.Output = &bytes.Buffer{}
	}

	if namespace == "" {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		namespace, _, _ = clientConfig.Namespace()
	}

	client, err := p.NewServingClient(namespace)
	if err != nil {
		return nil, p.Output, fmt.Errorf("failed to create new serving client: %v", err)
	}

	return client, p.Output, nil
}
