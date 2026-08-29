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
	"maps"
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
// of fields is disproportionate, and this project already has a precedent for
// reading Routes through the dynamic client (see pkg/pipelines/tekton/pac/pac.go
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
// router admitted it at. A Route this call created that fails admission is
// removed again; a pre-existing one is kept, it may be serving.
func (x *Exposer) Expose(ctx context.Context, client dynamic.Interface, e deployer.Exposure) (string, error) {
	route, err := x.generate(e)
	if err != nil {
		return "", fmt.Errorf("failed to generate Route: %w", err)
	}

	name, created, err := x.ensure(ctx, client, e, route)
	if err != nil {
		return "", err
	}

	host, err := waitForAdmitted(ctx, client, e.Namespace, name, admissionTimeout)
	if err != nil && created {
		delErr := client.Resource(routeGVR).Namespace(e.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if delErr != nil && !apierrors.IsNotFound(delErr) {
			return "", fmt.Errorf("%w; rolling the Route back failed too: %v", err, delErr)
		}
		return "", fmt.Errorf("%w; the Route was rolled back", err)
	}
	return host, err
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
// gauron99: I think we could simplify/improve this and IdentityLabels.
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
// find; a lookup that fails or is denied wraps deployer.ErrExposureNotVisible,
// so denial is never read as absence. Finding two managed Routes is a refusal
// of its own, not wrapped: the cluster answered, and the answer is ambiguous.
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
// With no domain, spec.host stays empty: the router mints
// "<name>-<namespace>.<domain>", and naming a host here would mean
// discovering the domain first. A --domain is the exception: it is used as
// the host verbatim, and the router admits or refuses the claim.
//
// The minted host's first label gives the function's name and namespace 63
// shared characters, fewer for keda, whose Route also carries the
// interceptor's namespace. Deliberately not checked up front: a check would
// encode OpenShift's minting template and refuse names a retemplated
// cluster accepts; the API server rejects an over-budget Route at creation
// and reports the real limit.
func (x *Exposer) generate(e deployer.Exposure) (*unstructured.Unstructured, error) {
	labels := maps.Clone(e.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[fnlabels.FunctionNamespaceKey] = e.FunctionNamespace

	annotations := maps.Clone(e.Annotations)
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[deployer.DeployerNameAnnotation] = x.deployerName

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
	if host := labels[deployer.DomainLabel]; host != "" {
		spec["host"] = host
	}

	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": routeGVR.GroupVersion().String(),
			"kind":       "Route",
			"spec":       spec,
		},
	}
	route.SetName(e.Name)
	route.SetNamespace(e.Namespace)
	route.SetLabels(labels)
	route.SetAnnotations(annotations)
	if e.Owner != nil {
		route.SetOwnerReferences([]metav1.OwnerReference{*e.Owner})
	}

	return route, nil
}

// ensure creates or updates the Route for e. The bool return is whether this
// call created the object: Expose rolls back only that object if admission
// fails. Find is by label, not by e.Name. Updates merge onto the live object
// (cert-manager state stays). A domain change is delete+create. A foreign
// Route on e.Name is AlreadyExists, never adopted. 409s retry; a controller
// status write can race.
func (x *Exposer) ensure(ctx context.Context, client dynamic.Interface,
	e deployer.Exposure, route *unstructured.Unstructured) (string, bool, error) {

	routes := client.Resource(routeGVR).Namespace(e.Namespace)

	existing, err := x.find(ctx, client, e.Ref())
	if err != nil {
		return "", false, err
	}

	if existing == nil {
		// create new Route
		route.SetResourceVersion("")
		if _, err := routes.Create(ctx, route, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return "", false, fmt.Errorf(
					"cannot expose function %q: a Route named %q already exists in namespace %q and was not created by func; rename or remove it, or deploy with --expose=none",
					e.FunctionName, e.Name, e.Namespace)
			}
			return "", false, fmt.Errorf("failed to create Route %q: %w", e.Name, err)
		}
		return e.Name, true, nil
	}

	// spec.host in place needs routes/custom-host (project admins lack it).
	// The injected cert is for the old host and must not follow the new one.
	if existing.GetLabels()[deployer.DomainLabel] != e.Labels[deployer.DomainLabel] {
		if err := routes.Delete(ctx, existing.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return "", false, fmt.Errorf("failed to replace Route %q for a changed domain: %w", existing.GetName(), err)
		}
		route.SetResourceVersion("")
		if _, err := routes.Create(ctx, route, metav1.CreateOptions{}); err != nil {
			return "", false, fmt.Errorf("failed to recreate Route %q for domain %q: %w", e.Name, e.Labels[deployer.DomainLabel], err)
		}
		return e.Name, true, nil
	}

	// Merge onto the live object so cert-manager keys and spec.tls PEM stay.
	// spec.host is left alone; domain changes already took delete+create.
	// A key we used to write and no longer generate lingers.
	name := existing.GetName()
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, getErr := routes.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		merged := current.DeepCopy()

		labels := merged.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		maps.Copy(labels, route.GetLabels())
		merged.SetLabels(labels)

		annotations := merged.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		maps.Copy(annotations, route.GetAnnotations())
		merged.SetAnnotations(annotations)

		for _, owned := range [][]string{
			{"spec", "to"},
			{"spec", "port"},
			{"spec", "tls", "termination"},
			{"spec", "tls", "insecureEdgeTerminationPolicy"},
			// The owner must track the current Service UID: after a Service
			// recreate, a Route keeping the old UID is garbage collected as
			// the dead Service's dependent. Absent on keda's ownerless
			// Exposure, so the copy below skips it there.
			{"metadata", "ownerReferences"},
		} {
			v, found, err := unstructured.NestedFieldCopy(route.Object, owned...)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			if err := unstructured.SetNestedField(merged.Object, v, owned...); err != nil {
				return err
			}
		}

		_, updateErr := routes.Update(ctx, merged, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return "", false, fmt.Errorf("failed to update Route %q: %w", name, err)
	}
	return name, false, nil
}

// isManaged is ours only if both the function label and this exposer's
// deployer annotation are set. Either alone is not enough. The deployer
// stamp is an annotation, so find cannot put it in the label selector.
func (x *Exposer) isManaged(route *unstructured.Unstructured) bool {
	return route.GetLabels()[fnlabels.FunctionKey] == "true" &&
		route.GetAnnotations()[deployer.DeployerNameAnnotation] == x.deployerName
}

// waitForAdmitted polls the Route status until any ingress entry (one per
// router shard) reports Admitted=True with a hostname. A False from one
// shard does not fail the wait while another entry has no terminal verdict
// (Unknown, missing Admitted, or True with no host yet). It fails early
// only when every reported shard has Admitted=False.
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

		var rejection error
		pending := false
		for _, raw := range ingresses {
			ingress, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			conditions, found, err := unstructured.NestedSlice(ingress, "conditions")
			if err != nil || !found {
				pending = true
				continue
			}
			verdict := ""
			for _, rawCond := range conditions {
				cond, ok := rawCond.(map[string]any)
				if !ok || cond["type"] != "Admitted" {
					continue
				}
				status, _ := cond["status"].(string)
				verdict = status
				switch status {
				case "True":
					host, _, _ = unstructured.NestedString(ingress, "host")
					if host == "" {
						// Admitted with no hostname is not a usable answer,
						// and returning it would hand the caller a bare
						// "https://" and record the function as exposed.
						lastErr = fmt.Errorf(
							"route %q was admitted by a router but reports no hostname", name)
						pending = true
						continue
					}
					return true, nil
				case "False":
					if rejection == nil {
						reason, _ := cond["reason"].(string)
						message, _ := cond["message"].(string)
						rejection = fmt.Errorf("route %q was rejected by the router: %s: %s", name, reason, message)
					}
				default:
					pending = true
				}
			}
			if verdict == "" {
				pending = true
			}
		}
		if rejection != nil && !pending {
			lastErr = rejection
			return false, rejection
		}
		if rejection != nil {
			lastErr = rejection
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
