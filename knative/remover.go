package knative

import (
	"context"
	"fmt"
	"time"
)

const RemoveTimeout = 120 * time.Second

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

func (remover *Remover) Remove(ctx context.Context, name string) (err error) {

	client, err := NewServingClient(remover.Namespace)
	if err != nil {
		return
	}

	fmt.Printf("Removing Knative Service: %v\n", name)

	err = client.DeleteService(name, RemoveTimeout)
	if err != nil {
		err = fmt.Errorf("knative remover failed to delete the service: %v", err)
	}

	return
}
