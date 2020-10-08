package knative

import (
	"bytes"
	"fmt"
	"time"

	"github.com/boson-project/faas/k8s"
)

func NewRemover(namespaceOverride string) (remover *Remover, err error) {
	remover = &Remover{}
	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	remover.Namespace = namespace

	return
}

type Remover struct {
	Namespace string
	Verbose   bool
}

func (remover *Remover) Remove(name string) (err error) {

	serviceName, err := k8s.ToK8sAllowedName(name)
	if err != nil {
		return
	}

	client, output, err := NewServingClient(remover.Namespace, remover.Verbose)
	if err != nil {
		return
	}

	err = client.DeleteService(serviceName, time.Second*60)
	if err != nil {
		if remover.Verbose {
			err = fmt.Errorf("remover failed to delete the service: %v", err)
		} else {
			err = fmt.Errorf("remover failed to delete the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
		}
	}

	return
}
