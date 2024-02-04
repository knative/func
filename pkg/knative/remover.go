package knative

import (
	"context"
	"fmt"
	"os"
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
	if ns == "" {
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in knative remover\n")
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
