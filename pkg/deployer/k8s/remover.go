package k8s

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
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in knative remover\n")
		return fn.ErrNamespaceRequired
	}

	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(ns)
	serviceClient := clientset.CoreV1().Services(ns)

	err = deploymentClient.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return fn.ErrFunctionNotFound
		}
		return fmt.Errorf("k8s remover failed to delete the deployment: %v", err)
	}

	err = serviceClient.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return fn.ErrFunctionNotFound
		}
		return fmt.Errorf("k8s remover failed to delete the service: %v", err)
	}

	return nil
}
