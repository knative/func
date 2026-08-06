/*
Package ocproute exposes a function through an OpenShift Route.

Routes are an OpenShift-only resource, but both binaries attach this Exposer on
every platform: on a cluster with no Route API the cost is one List that comes
back NotFound. Choosing it is the caller's job: nothing here detects the
platform.
*/
package ocproute

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"knative.dev/func/pkg/deployer"
	fnlabels "knative.dev/func/pkg/k8s/labels"
)

// routeGVR identifies the OpenShift Route resource. No typed client is used
// here: adding github.com/openshift/api as a direct dependency for a handful
// of fields is disproportionate, and this project already has a precedent for reading Routes
// through the dynamic client (see pkg/pipelines/tekton/pac/pac.go
// DetectPACOpenShiftRoute). Route's structure is also small and stable (a v1,
// GA API since OpenShift 3.x), so hand-built unstructured content carries little
// maintenance risk.
var routeGVR = schema.GroupVersionResource{
	Group:    "route.openshift.io",
	Version:  "v1",
	Resource: "routes",
}

// admissionTimeout bounds the wait for a router to accept a Route. A router
// that has not answered in this long is not going to.
const admissionTimeout = 30 * time.Second

// Exposer creates and removes the OpenShift Route fronting a function.
type Exposer struct {
	// deployerName goes onto every Route this Exposer creates and is
	// checked again before deleting one, so the Route minted for a keda
	// function is never removed by the raw deployer, or the other way
	// round.
	deployerName string
}

// New returns an Exposer stamping its Routes with deployerName, one of the
// names in pkg/deployers.
func New(deployerName string) *Exposer {
	return &Exposer{deployerName: deployerName}
}

// Expose creates or updates the Route for 'e' and returns the hostname a
// router admitted it at.
func (x *Exposer) Expose(ctx context.Context, client dynamic.Interface, e deployer.Exposure) (string, error) {
	route, err := x.generate(e)
	if err != nil {
		return "", fmt.Errorf("failed to generate Route: %w", err)
	}

	name, err := x.ensure(ctx, client, e, route)
	if err != nil {
		return "", err
	}

	return waitForAdmitted(ctx, client, e.Namespace, name, admissionTimeout)
}

// Unexpose deletes the Route belonging to the function named by ref, leaving
// in place any Route this Exposer did not create.
func (x *Exposer) Unexpose(ctx context.Context, client dynamic.Interface, ref deployer.ExposureRef) error {
	route, err := x.find(ctx, client, ref)
	if err != nil || route == nil {
		return err
	}

	err = client.Resource(routeGVR).Namespace(ref.Namespace).Delete(ctx, route.GetName(), metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Route %q: %w", route.GetName(), err)
	}
	return nil
}

// selector matches the Routes this Exposer creates for one function. The
// deployer stamp is deliberately not in it: that is an annotation, which no
// selector can filter on, so find() applies it afterwards through isManaged.
func selector(ref deployer.ExposureRef) string {
	return k8slabels.SelectorFromSet(k8slabels.Set{
		fnlabels.FunctionKey:          "true",
		fnlabels.FunctionNameKey:      ref.FunctionName,
		fnlabels.FunctionNamespaceKey: ref.FunctionNamespace,
	}).String()
}

// find returns the Route this Exposer manages for the function named by ref,
// or nil when there is none. Lookup is by label: labels are what func stamped
// on the Route it created. A missing Route API or namespace means nothing to
// find; any other failure wraps deployer.ErrExposureNotVisible, so denial is
// never read as absence.
func (x *Exposer) find(ctx context.Context, client dynamic.Interface, ref deployer.ExposureRef) (*unstructured.Unstructured, error) {
	list, err := client.Resource(routeGVR).Namespace(ref.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: selector(ref)})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: looking for the Route of function %q: %w",
			deployer.ErrExposureNotVisible, ref.FunctionName, err)
	}

	var found *unstructured.Unstructured
	for i := range list.Items {
		if !x.isManaged(&list.Items[i]) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf(
				"found %d Routes for function %q in namespace %q (%q and %q); func cannot tell which one it should manage, remove the stale one by hand",
				len(list.Items), ref.FunctionName, ref.Namespace, found.GetName(), list.Items[i].GetName())
		}
		found = &list.Items[i]
	}
	return found, nil
}

// generate builds, but does not create, the Route exposing e's target Service.
// spec.host stays empty: the router mints "<name>-<namespace>.<domain>", and
// naming a host here would mean discovering the domain first.
//
// The minted first label caps name+namespace at 63 chars combined (measured:
// 62 raw, 47 keda under CMA). The API server rejects an over-budget Route at
// creation, not at admission. Deliberately not checked up front: a check
// would encode OpenShift's default minting template and refuse names a
// retemplated cluster accepts; the API server reports the real limit.
func (x *Exposer) generate(e deployer.Exposure) (*unstructured.Unstructured, error) {
	labels, err := deployer.GenerateCommonLabels(e.Function, e.Decorator)
	if err != nil {
		return nil, err
	}

	labels[fnlabels.FunctionNamespaceKey] = e.FunctionNamespace

	annotations := deployer.GenerateCommonAnnotations(e.Function, e.Decorator, false /* dapr n/a for routing */, x.deployerName)

	metadata := map[string]any{
		"name":        e.Name,
		"namespace":   e.Namespace,
		"labels":      stringMapToAny(labels),
		"annotations": stringMapToAny(annotations),
	}
	if e.Owner != nil {
		metadata["ownerReferences"] = []any{ownerReferenceToAny(*e.Owner)}
	}

	spec := map[string]any{
		"to": map[string]any{
			"kind": "Service",
			"name": e.TargetService,
		},
		"port": map[string]any{
			"targetPort": e.TargetPort,
		},
		// Edge TLS via the router's wildcard cert - zero cert
		// management; Redirect upgrades http requests to https.
		"tls": map[string]any{
			"termination":                   "edge",
			"insecureEdgeTerminationPolicy": "Redirect",
		},
	}
	// A custom domain is used verbatim as the host. DNS and the certificate
	// are the user's: point DNS at the router, and have something like
	// cert-manager inject the cert (ensure carries it over). The router
	// reports a host collision at admission.
	if e.Function.Domain != "" {
		spec["host"] = e.Function.Domain
	}

	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": routeGVR.GroupVersion().String(),
			"kind":       "Route",
			"metadata":   metadata,
			"spec":       spec,
		},
	}

	return route, nil
}

// stringMapToAny converts a map[string]string to the map[string]any
// unstructured.Unstructured needs its nested fields to be.
func stringMapToAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ownerReferenceToAny converts an OwnerReference to the map[string]any
// unstructured.Unstructured needs its nested fields to be.
func ownerReferenceToAny(ref metav1.OwnerReference) map[string]any {
	out := map[string]any{
		"apiVersion": ref.APIVersion,
		"kind":       ref.Kind,
		"name":       ref.Name,
		"uid":        string(ref.UID),
	}
	if ref.Controller != nil {
		out["controller"] = *ref.Controller
	}
	return out
}

// ensure creates or updates the Route for e, returning the name it settled
// on. An existing managed Route is found by label and updated under whatever
// name it carries, so a naming-scheme change cannot strand one. A foreign
// Route occupying the wanted name is never adopted or overwritten; the
// create's AlreadyExists is reported instead. Updates retry on 409, since a
// controller status write can race.
func (x *Exposer) ensure(ctx context.Context, client dynamic.Interface, e deployer.Exposure, route *unstructured.Unstructured) (string, error) {
	routes := client.Resource(routeGVR).Namespace(e.Namespace)

	existing, err := x.find(ctx, client, e.Ref())
	if err != nil {
		return "", err
	}

	if existing == nil {
		// create new Route
		route.SetResourceVersion("")
		if _, err := routes.Create(ctx, route, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return "", fmt.Errorf(
					"cannot expose function %q: a Route named %q already exists in namespace %q and was not created by func; rename or remove it, or deploy with --expose=none",
					e.Function.Name, e.Name, e.Namespace)
			}
			return "", fmt.Errorf("failed to create Route %q: %w", e.Name, err)
		}
		return e.Name, nil
	}

	// A changed domain means a changed spec.host. Updating that in place
	// needs "update" on routes/custom-host, which project admins lack by
	// default; delete+create needs only "create", which they have. So the
	// Route is recreated. The injected certificate, if any, dies with it:
	// it named the old host.
	if existing.GetLabels()[deployer.DomainLabel] != e.Function.Domain {
		if err := routes.Delete(ctx, existing.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to replace Route %q for a changed domain: %w", existing.GetName(), err)
		}
		route.SetResourceVersion("")
		if _, err := routes.Create(ctx, route, metav1.CreateOptions{}); err != nil {
			return "", fmt.Errorf("failed to recreate Route %q for domain %q: %w", e.Name, e.Function.Domain, err)
		}
		return e.Name, nil
	}

	// update existing Route
	name := existing.GetName()
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, getErr := routes.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		route.SetName(name)
		route.SetResourceVersion(current.GetResourceVersion())
		// Carry over TLS material this Exposer did not author: a certificate
		// controller (e.g. cert-manager's openshift-routes) writes the cert
		// for a custom domain into spec.tls, and regenerating the spec must
		// not wipe it on every redeploy.
		if tls, found, _ := unstructured.NestedMap(current.Object, "spec", "tls"); found {
			for _, k := range []string{"certificate", "key", "caCertificate", "destinationCACertificate"} {
				if v, ok := tls[k]; ok {
					if err := unstructured.SetNestedField(route.Object, v, "spec", "tls", k); err != nil {
						return err
					}
				}
			}
		}
		_, updateErr := routes.Update(ctx, route, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to update Route %q: %w", name, err)
	}
	return name, nil
}

// isManaged reports whether route was created by this Exposer - as opposed
// to a user-authored or third-party Route, which must never be touched.
// Both signals are required: a bare boson.dev/function label, or a deployer
// annotation written by some other component, alone does not prove ownership.
// The deployer name is checked here rather than in the selector because it is
// an annotation, and no label selector can filter on those.
func (x *Exposer) isManaged(route *unstructured.Unstructured) bool {
	return route.GetLabels()[fnlabels.FunctionKey] == "true" &&
		route.GetAnnotations()[deployer.DeployerNameAnnotation] == x.deployerName
}

// waitForAdmitted polls the Route status until any ingress entry (one per
// router shard) reports Admitted=True, returning that entry's host. Admitted
// with no hostname yet keeps polling. It fails early only when a shard
// reports Admitted=False and none reports True; entries with no verdict poll
// through to the timeout.
func waitForAdmitted(ctx context.Context, client dynamic.Interface, ns, name string, timeout time.Duration) (string, error) {
	routes := client.Resource(routeGVR).Namespace(ns)

	var host string
	var lastErr error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		route, err := routes.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = fmt.Errorf("failed to get Route %q: %w", name, err)
			return false, nil
		}

		ingresses, found, err := unstructured.NestedSlice(route.Object, "status", "ingress")
		if err != nil || !found {
			return false, nil
		}

		// Scan every entry before deciding: verdicts are per router, and a
		// rejection by one shard must not override an admission by another.
		var rejection error
		for _, raw := range ingresses {
			ingress, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			conditions, found, err := unstructured.NestedSlice(ingress, "conditions")
			if err != nil || !found {
				continue
			}
			for _, rawCond := range conditions {
				cond, ok := rawCond.(map[string]any)
				if !ok || cond["type"] != "Admitted" {
					continue
				}
				status, _ := cond["status"].(string)
				switch status {
				case "True":
					host, _, _ = unstructured.NestedString(ingress, "host")
					if host == "" {
						// Admitted with no hostname is not a usable answer,
						// and returning it would hand the caller a bare
						// "https://" and record the function as exposed.
						// Another entry may still carry one; otherwise keep
						// polling and say what happened if it never appears.
						lastErr = fmt.Errorf(
							"route %q was admitted by a router but reports no hostname", name)
						continue
					}
					return true, nil
				case "False":
					if rejection == nil {
						reason, _ := cond["reason"].(string)
						message, _ := cond["message"].(string)
						rejection = fmt.Errorf("route %q was rejected by the router: %s: %s", name, reason, message)
					}
				}
				// Unknown or missing status: keep polling.
			}
		}
		if rejection != nil {
			lastErr = rejection
			return false, rejection
		}
		return false, nil
	})
	if pollErr != nil {
		if lastErr != nil {
			return "", lastErr
		}
		return "", fmt.Errorf("route %q was not admitted by any router within %s: %w", name, timeout, pollErr)
	}
	return host, nil
}
