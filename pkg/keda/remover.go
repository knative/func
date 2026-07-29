package keda

import (
	"context"
	"fmt"
	"os"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type RemoverOpt func(*Remover)

func NewRemover(verbose bool, opts ...RemoverOpt) *Remover {
	r := &Remover{
		verbose: verbose,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithRemoverExposer gives the Remover the mechanism that exposed the
// function, so Remove can take that exposure away. A keda function's exposing
// object lives in the interceptor's namespace and carries no owner reference,
// since Kubernetes rejects one across namespaces, so deleting the Deployment
// does not garbage collect it the way it collects everything else.
func WithRemoverExposer(exposer deployer.Exposer) RemoverOpt {
	return func(r *Remover) {
		r.exposer = exposer
	}
}

type Remover struct {
	verbose bool
	exposer deployer.Exposer
}

func (remover *Remover) Remove(ctx context.Context, name, ns string, f fn.Function) error {
	if ns == "" {
		fmt.Fprintf(os.Stderr, "no namespace defined when trying to delete a function in keda remover\n")
		return fn.ErrNamespaceRequired
	}

	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	svc, err := clientset.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	switch {
	case apiErrors.IsNotFound(err):
		// No Service means nothing to inspect: the record lived on it, and so
		// did the annotation saying this was keda's.
		//
		// A Route may survive this, since keda's carries no owner reference and
		// nothing collects it. That leftover is manual cleanup: finding it
		// would mean guessing at a namespace nobody recorded.
		return fn.ErrNotHandled

	case err != nil:
		return err

	case !UsesKedaDeployer(svc.Annotations):
		// Another deployer's function, and its own objects are all still here
		// to say so.
		return fn.ErrNotHandled
	}

	// WHERE the Route is, read from the Service BEFORE anything is deleted
	recordedNS := svc.Annotations[k8s.RouteNamespaceAnnotation]

	// Remove the Route before deleting anything, and fail the whole delete if it
	// cant be removed. Failing here touches nothing, so the user can simply retry
	if remover.exposer != nil {
		dynClient, err := k8s.NewDynamicClient()
		if err != nil {
			return fmt.Errorf("could not setup dynamic client: %w", err)
		}
		if err := remover.unexpose(ctx, dynClient, recordedNS, name, ns); err != nil {
			return err
		}
	}

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

	return nil
}

// unexpose removes the function's Route. A non-empty recordedNS names the
// namespace the deploy created the Route in and is acted on as written:
// nothing probed, no namespace guessed. An empty recordedNS means no deploy
// recorded a Route; every interceptor candidate namespace is swept anyway,
// since keda's Route has no owner reference and delete is the last chance to
// catch one left unrecorded by a crash. A candidate that cannot be listed is
// never read as empty. dynClient is a parameter so a test can reach this.
func (remover *Remover) unexpose(ctx context.Context, dynClient dynamic.Interface, recordedNS, name, ns string) error {
	if recordedNS == "" {
		for _, candidate := range interceptorNamespaceCandidates() {
			if err := remover.exposer.Unexpose(ctx, dynClient, interceptorExposureRef(name, ns, candidate)); err != nil {
				return fmt.Errorf("no record said where this function's Route was, and namespace %q "+
					"could not be checked for one: %w", candidate, err)
			}
		}
		return nil
	}

	if err := remover.exposer.Unexpose(ctx, dynClient, interceptorExposureRef(name, ns, recordedNS)); err != nil {
		return fmt.Errorf("could not remove the Route exposing function %q in namespace %q; "+
			"nothing was deleted and the function is still running, if you fix this you can run delete again: %w",
			name, recordedNS, err)
	}
	return nil
}
