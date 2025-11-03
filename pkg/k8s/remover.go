package k8s

import (
	"context"
	"fmt"
	"os"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
)

func NewRemover(verbose bool) *Remover {
	return &Remover{
		verbose: verbose,
	}
}

type Remover struct {
	verbose bool
}

func (remover *Remover) Remove(ctx context.Context, name, ns string) (bool, error) {
	if ns == "" {
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in knative remover\n")
		return false, fn.ErrNamespaceRequired
	}

	clientset, err := NewKubernetesClientset()
	if err != nil {
		return false, fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	serviceClient := clientset.CoreV1().Services(ns)
	svc, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return false, fn.ErrFunctionNotFound
		}
		return false, err
	}

	if UsesRawDeployer(svc.Annotations) {
		// if annotation is set and the deployer name is set explicitly to the raw deployer, we need to handle this service

		deploymentClient := clientset.AppsV1().Deployments(ns)

		// TODO: delete only one and let the api server handle the other via the owner reference
		err = deploymentClient.Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			if apiErrors.IsNotFound(err) {
				return true, fn.ErrFunctionNotFound
			}
			return true, fmt.Errorf("k8s remover failed to delete the deployment: %v", err)
		}

		err = serviceClient.Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			if apiErrors.IsNotFound(err) {
				return true, fn.ErrFunctionNotFound
			}
			return true, fmt.Errorf("k8s remover failed to delete the service: %v", err)
		}

		return true, nil
	}
	return false, nil
}
