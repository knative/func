package k8s

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// routeGVR identifies the OpenShift Route resource. No typed client is used
// here: adding github.com/openshift/api as a direct dependency for a
// handful of fields is disproportionate, this project already has a
// precedent for reading Routes through the dynamic client (see
// pkg/pipelines/tekton/pac/pac.go DetectPACOpenShiftRoute), and there is no
// existing github.com/openshift/api requirement anywhere in go.mod to build
// on. Route's structure is also small and stable (a v1, GA API since
// OpenShift 3.x), so hand-built unstructured content carries little
// maintenance risk.
var routeGVR = schema.GroupVersionResource{
	Group:    "route.openshift.io",
	Version:  "v1",
	Resource: "routes",
}

// GenerateRoute builds (but does not create) the OpenShift Route that
// exposes svcName's "http" port. spec.host is left empty so the cluster's
// router mints one (see docs/research citations in the openshift-route-fork
// records) - custom domains are out of scope for this commit.
func GenerateRoute(f fn.Function, svcName string, deployment *appsv1.Deployment, decorator deployer.DeployDecorator, deployerName string) (*unstructured.Unstructured, error) {
	labels, err := deployer.GenerateCommonLabels(f, decorator)
	if err != nil {
		return nil, err
	}
	annotations := deployer.GenerateCommonAnnotations(f, decorator, false /* dapr n/a for routing */, deployerName)

	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": routeGVR.GroupVersion().String(),
			"kind":       "Route",
			"metadata": map[string]any{
				"name":        f.Name,
				"namespace":   deployment.Namespace,
				"labels":      stringMapToAny(labels),
				"annotations": stringMapToAny(annotations),
				"ownerReferences": []any{
					map[string]any{
						"apiVersion": appsv1.SchemeGroupVersion.WithKind("Deployment").GroupVersion().String(),
						"kind":       "Deployment",
						"name":       deployment.Name,
						"uid":        string(deployment.UID),
						"controller": true,
					},
				},
			},
			"spec": map[string]any{
				"to": map[string]any{
					"kind": "Service",
					"name": svcName,
				},
				"port": map[string]any{
					"targetPort": "http",
				},
				// Edge TLS via the router's wildcard cert - zero cert
				// management; Redirect upgrades http requests to https.
				"tls": map[string]any{
					"termination":                   "edge",
					"insecureEdgeTerminationPolicy": "Redirect",
				},
			},
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

// EnsureRoute creates or updates a Route, retrying the whole
// get-mutate-update cycle on a 409 conflict (a controller status write can
// race an update from here).
func EnsureRoute(ctx context.Context, dynClient dynamic.Interface, ns string, route *unstructured.Unstructured) error {
	client := dynClient.Resource(routeGVR).Namespace(ns)
	name := route.GetName()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, getErr := client.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				route.SetResourceVersion("")
				_, createErr := client.Create(ctx, route, metav1.CreateOptions{})
				return createErr
			}
			return getErr
		}
		route.SetResourceVersion(existing.GetResourceVersion())
		_, updateErr := client.Update(ctx, route, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return fmt.Errorf("failed to ensure Route %q: %w", name, err)
	}
	return nil
}

// isManagedRoute reports whether route was created by GenerateRoute() - as
// opposed to a user-authored or third-party Route that happens to share the
// function's name, which must never be deleted out from under the user.
// Both signals are required: a bare boson.dev/function label, or a
// deployer annotation written by some other component, alone does not
// prove func's raw deployer owns the route.
func isManagedRoute(route *unstructured.Unstructured) bool {
	return route.GetLabels()["boson.dev/function"] == "true" &&
		route.GetAnnotations()[deployer.DeployerNameAnnotation] == KubernetesDeployerName
}

// RemoveManagedRoute deletes the Route named 'name' in 'ns' only if func
// owns it (isManagedRoute()). Returns (removed, error):
//   - not found (route absent, or the Route API isn't installed) -> (false, nil)
//   - found but not managed -> (false, nil), warning printed, route kept
//   - found and managed, deleted -> (true, nil)
func RemoveManagedRoute(ctx context.Context, dynClient dynamic.Interface, ns, name string) (bool, error) {
	client := dynClient.Resource(routeGVR).Namespace(ns)

	route, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check for existing Route %q: %w", name, err)
	}

	if !isManagedRoute(route) {
		fmt.Fprintf(os.Stderr,
			"⚠️  a Route named %q exists in namespace %q but is not managed by func - leaving it in place\n",
			name, ns)
		return false, nil
	}

	if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("failed to delete Route %q: %w", name, err)
	}
	return true, nil
}

// WaitForRouteAdmitted polls the Route status until any ingress entry (one
// per router/IngressController shard - a cluster can run more than one)
// reports Admitted=True, returning that entry's host. It fails immediately
// (not waiting out the full timeout) only when an ingress entry explicitly
// reports Admitted=False - e.g. a host already claimed by another Route -
// surfacing the condition's reason and message. An entry with no Admitted
// condition yet is polled through to the timeout, fail-open on unknown.
func WaitForRouteAdmitted(ctx context.Context, dynClient dynamic.Interface, ns, name string, timeout time.Duration) (string, error) {
	client := dynClient.Resource(routeGVR).Namespace(ns)

	var host string
	var lastErr error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		route, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = fmt.Errorf("failed to get Route %q: %w", name, err)
			return false, nil
		}

		ingresses, found, err := unstructured.NestedSlice(route.Object, "status", "ingress")
		if err != nil || !found {
			return false, nil
		}

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
					return true, nil
				case "False":
					reason, _ := cond["reason"].(string)
					message, _ := cond["message"].(string)
					lastErr = fmt.Errorf("route %q was rejected by the router: %s: %s", name, reason, message)
					return false, lastErr
				}
				// Unknown or missing status: keep polling.
			}
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

// GetAdmittedRouteHost is a single, non-blocking read of a Route's currently
// admitted host, for display paths (describe/list) that must return
// immediately rather than poll like WaitForRouteAdmitted does. Returns
// ("", false, nil) if the Route doesn't exist or has no Admitted=True
// ingress entry yet - both are "no external URL to show", not errors.
func GetAdmittedRouteHost(ctx context.Context, dynClient dynamic.Interface, ns, name string) (string, bool, error) {
	route, err := dynClient.Resource(routeGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get Route %q: %w", name, err)
	}

	ingresses, found, err := unstructured.NestedSlice(route.Object, "status", "ingress")
	if err != nil || !found {
		return "", false, nil
	}
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
			if status, _ := cond["status"].(string); status == "True" {
				host, _, _ := unstructured.NestedString(ingress, "host")
				return host, host != "", nil
			}
		}
	}
	return "", false, nil
}
