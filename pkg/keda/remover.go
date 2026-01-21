package keda

import (
	"context"
	"fmt"
	"os"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

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
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in keda remover\n")
		return fn.ErrNamespaceRequired
	}

	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	serviceClient := clientset.CoreV1().Services(ns)
	svc, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Service doesn't exist - we don't handle this
			return fn.ErrNotHandled
		}
		return err
	}

	if !UsesKedaDeployer(svc.Annotations) {
		return fn.ErrNotHandled
	}

	// We're responsible, for this function --> proceed...

	deploymentClient := clientset.AppsV1().Deployments(ns)

	// delete only the deployment and let the api server handle the others via the owner reference
	err = deploymentClient.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return fn.ErrFunctionNotFound
		}
		return fmt.Errorf("keda remover failed to delete the deployment: %v", err)
	}

	if err := k8s.WaitForServiceRemoved(ctx, clientset, ns, name, k8s.DefaultWaitingTimeout); err != nil {
		return fmt.Errorf("k8s remover failed to propagate service deletion: %v", err)
	}

	return nil
}
