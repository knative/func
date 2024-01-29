package knative

import (
	"context"
	"fmt"
	"time"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	fn "knative.dev/func/pkg/functions"
)

const RemoveTimeout = 120 * time.Second

func NewRemover(verbose bool) *Remover {
	return &Remover{
		verbose: verbose,
	}
}

type Remover struct {
	verbose bool
}

func (remover *Remover) Remove(ctx context.Context, name, ns string) (err error) {
	// if namespace is not provided for any reason, use the active namespace
	// otherwise just throw an error. I dont think this should default to any namespace
	// because its a remover, therefore we dont want to just assume a default namespace
	// to delete a function from. Use provided, get the current one or none at all.

	if ns == "" {
		ns = ActiveNamespace()
	}

	if ns == "" {
		fmt.Print("normal error in Remove, no namespace found here\n")
		return fn.ErrNamespaceRequired
	}

	client, err := NewServingClient(ns)
	if err != nil {
		return
	}

	err = client.DeleteService(ctx, name, RemoveTimeout)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return fn.ErrFunctionNotFound
		}
		err = fmt.Errorf("knative remover failed to delete the service: %v", err)
	}

	return
}
