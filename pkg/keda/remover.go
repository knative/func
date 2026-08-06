package keda

import (
	"context"
	"fmt"
	"os"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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

	// Routes exist only on OpenShift. Elsewhere dynClient stays nil and remove
	// skips the Route entirely.
	var dynClient dynamic.Interface
	if k8s.IsOpenShift() {
		if dynClient, err = k8s.NewDynamicClient(); err != nil {
			return fmt.Errorf("could not setup dynamic client: %w", err)
		}
	}

	return remove(ctx, clientset, dynClient, name, ns)
}

// remove deletes the function: Deployment first, interceptor Route last. A nil
// dynClient means the cluster has no Route API, so there is no Route to remove.
//
// Route is the only resource not Garbage-Collected through Deployment's owner
// reference. Route lives in different namespace. Route delete runs last just so
// we don't gate function removal on insufficient Route RBAC grants or similar.
//
// Removal does not depend on the current expose setting, so a Route left by an
// earlier deploy goes too. It is found by exact name, derived from the function
// name and namespace, in the interceptor namespace as resolved now, and deleted
// only if it carries func's managed label. Anything outside that is left alone.
func remove(ctx context.Context, clientset kubernetes.Interface, dynClient dynamic.Interface, name, ns string) error {
	deploymentClient := clientset.AppsV1().Deployments(ns)

	// delete only the deployment and let the api server handle the others via the owner reference
	if err := deploymentClient.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apiErrors.IsNotFound(err) {
			return fn.ErrFunctionNotFound
		}
		return fmt.Errorf("keda remover failed to delete the deployment: %v", err)
	}

	if err := k8s.WaitForServiceRemoved(ctx, clientset, ns, name, k8s.DefaultWaitingTimeout); err != nil {
		return fmt.Errorf("k8s remover failed to propagate service deletion: %v", err)
	}

	if dynClient != nil {
		if err := removeInterceptorRoute(ctx, dynClient, name, ns); err != nil {
			return err
		}
	}

	return nil
}
