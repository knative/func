package deployer

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// ErrExposureNotVisible means the Exposer could not tell whether an exposing
// object exists, not that none does. Denial is the usual cause: an account with
// no rights in the object's namespace gets the same answer either way.
//
// The two need opposite handling. A caller removing what the user asked to
// remove must not report success on this; a caller reconciling a function that
// never mentioned exposure must not fail on it.
var ErrExposureNotVisible = errors.New("cannot determine whether an exposing object exists")

// ExposureRef identifies one function's exposing object without describing how
// to build it. Teardown needs only this, and finds the object by the labels
// these fields become: func looks for the object it labelled, not for whatever
// sits at a name it would have chosen.
type ExposureRef struct {
	// Both are needed: where one namespace collects every function's objects,
	// the name alone cannot separate two functions of the same name.
	FunctionName      string
	FunctionNamespace string

	// Namespace holds the exposing object, not always the function's own:
	// keda targets the interceptor Service in the operator's namespace, and an
	// exposing object can only target a Service beside it.
	Namespace string
}

// NewExposureRef is the identity of one function's exposing object: the
// function's name and namespace, and the namespace holding the object.
func NewExposureRef(functionName, functionNamespace, namespace string) ExposureRef {
	return ExposureRef{
		FunctionName:      functionName,
		FunctionNamespace: functionNamespace,
		Namespace:         namespace,
	}
}

// Exposure describes the external address wanted for one function: which
// Service to send traffic to, and what to name the object that does it.
// Labels and Annotations are the stamps the deployer already built; the
// exposer does not see fn.Function.
type Exposure struct {
	// Name of the function being exposed: the identity for lookup, like
	// FunctionNamespace.
	FunctionName string

	// Where the function is deployed, which is not always where its exposing
	// object goes. See ExposureRef.
	FunctionNamespace string

	// Name of the exposing object, not always the function's. Where one
	// namespace collects every function's objects it must carry the function's
	// namespace too, or same-named functions collide. Creation uses it; lookup
	// goes by label.
	Name string

	// Namespace holds the exposing object. See ExposureRef.
	Namespace string

	// TargetService is the Service to send traffic to, TargetPort the port name
	// on it. The raw deployer targets the function's own Service on "http";
	// keda targets the interceptor on "proxy".
	TargetService string
	TargetPort    string

	// Owner is deleted together with the exposing object. Nil where the two
	// cannot be linked, since Kubernetes rejects an owner reference across
	// namespaces, which obliges the caller to Unexpose by hand.
	Owner *metav1.OwnerReference

	Labels      map[string]string
	Annotations map[string]string
}

// Ref is what identifies this Exposure's object on the cluster, as opposed to
// what describes how to build it.
func (e Exposure) Ref() ExposureRef {
	return NewExposureRef(e.FunctionName, e.FunctionNamespace, e.Namespace)
}

// Exposer gives a function an address reachable from outside the cluster, one
// implementation per mechanism. A deployer with no Exposer leaves its functions
// cluster-local, which is the default.
//
// The dynamic client is a parameter, not a field: constructing an Exposer must
// not need a kubeconfig, because every command builds the deployer, including
// those that never reach a cluster.
type Exposer interface {
	// Expose creates or updates the exposing object and returns the hostname
	// a router admitted it at, waiting for that admission.
	Expose(ctx context.Context, client dynamic.Interface, e Exposure) (host string, err error)

	// Unexpose removes the object belonging to ref's function. It takes a
	// ref, not a name, so it removes the object func labelled and leaves
	// anything else alone. Finding nothing to remove is success.
	Unexpose(ctx context.Context, client dynamic.Interface, ref ExposureRef) error
}
