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

func (remover *Remover) Remove(ctx context.Context, name, ns string) error {
	if ns == "" {
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in knative remover\n")
		return fn.ErrNamespaceRequired
	}

	client, err := NewServingClient(ns)
	if err != nil {
		return err
	}

	ksvc, err := client.GetService(ctx, name)
	if err != nil {
		// If we can't get the service, check why
		if IsCRDNotFoundError(err) {
			// Knative Serving not installed - we don't handle this
			return fn.ErrNotHandled
		}
		if apiErrors.IsNotFound(err) {
			// Service doesn't exist as a Knative service - we don't handle this
			return fn.ErrNotHandled
		}
		// Some other error (permissions, network, etc.) - this is a real error
		// We can't determine if we should handle it, so propagate it
		return fmt.Errorf("failed to get knative service: %w", err)
	}

	if !UsesKnativeDeployer(ksvc.Annotations) {
		return fn.ErrNotHandled
	}

	// We're responsible, for this function --> proceed...

	err = client.DeleteService(ctx, name, RemoveTimeout)
	if err != nil {
		return fmt.Errorf("knative remover failed to delete the service: %v", err)
	}

	return nil
}
