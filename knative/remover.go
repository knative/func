package knative

import (
	"bytes"
	"fmt"
	"github.com/boson-project/faas/k8s"
	"io"
	"k8s.io/client-go/tools/clientcmd"
	commands "knative.dev/client/pkg/kn/commands"
	"os"
	"time"
)

func NewRemover(namespaceOverride string) *Remover {
	return &Remover{Namespace: namespaceOverride}
}

type Remover struct {
	Namespace string
	Verbose   bool
}

func (remover *Remover) Remove(name string) (err error) {

	project, err := k8s.ToK8sAllowedName(name)
	if err != nil {
		return
	}

	var output io.Writer
	if remover.Verbose {
		output = os.Stdout
	} else {
		output = &bytes.Buffer{}
	}

	p := commands.KnParams{}
	p.Initialize()
	p.Output = output

	if err != nil {
		return err
	}
	if remover.Namespace == "" {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		remover.Namespace, _, _ = clientConfig.Namespace()
	}
	client, err := p.NewServingClient(remover.Namespace)
	if err != nil {
		return fmt.Errorf("remover failed to create new serving client: %v", err)
	}

	err = client.DeleteService(project, time.Second*30)
	if err != nil {
		if remover.Verbose {
			err = fmt.Errorf("remover failed to delete the service: %v", err)
		} else {
			err = fmt.Errorf("remover failed to delete the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
		}
	}

	return
}
