package knative

import (
	"bytes"
	"fmt"
	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
	"io"
	commands "knative.dev/client/pkg/kn/commands"
	"os"
	"time"
)

func NewRemover() *Remover {
	return &Remover{Namespace: faas.DefaultNamespace}
}

type Remover struct {
	Namespace string
	Verbose   bool
}

func (remover *Remover) Remove(name string) (err error) {

	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return
	}

	output := io.Writer(nil)
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
