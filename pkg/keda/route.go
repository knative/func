package keda

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	// Where the keda http-add-on installs its interceptor. The OpenShift
	// Custom Metrics Autoscaler operator uses openshift-keda; the upstream
	// helm chart, which is what pkg/cluster/keda.go and hack/cluster.sh set up
	// on KinD, uses keda. Call interceptorNamespace() rather than picking one.
	interceptorNamespaceOpenShift = "openshift-keda"
	interceptorNamespaceUpstream  = "keda"

	interceptorServiceName = "keda-add-ons-http-interceptor-proxy"
	// interceptorServicePortName is the interceptor Service's own port name.
	// It is "proxy", not "http" like the function's own Service uses.
	interceptorServicePortName = "proxy"
)

// interceptorNamespace returns the namespace the interceptor runs in on this
// cluster. A cluster has exactly one - the same Service backs both the
// cluster-local bridge and the OpenShift Route - so every caller resolves it
// here rather than assuming an install method.
func interceptorNamespace() string {
	if k8s.IsOpenShift() {
		return interceptorNamespaceOpenShift
	}
	return interceptorNamespaceUpstream
}

// interceptorRouteName builds the Route name for a function. Every keda
// function's Route shares one interceptor namespace, so the name carries the
// function's namespace too, or two functions of the same name would collide.
func interceptorRouteName(name, namespace string) string {
	return fmt.Sprintf("%s-%s", name, namespace)
}

// generateInterceptorRoute returns the Route object exposing the shared keda
// interceptor. ensureInterceptorRoute is what sends it to the cluster.
// It targets the interceptor's own Service - not the function's Service, and
// not the per-function ExternalName bridge. Route's spec.to has no namespace
// field (openshift/api route/v1 RouteTargetReference is Kind/Name/Weight only),
// so a Route can only target a Service in its own namespace.
//
// No ownerRef: Kubernetes rejects cross-namespace owner references, so this
// Route cannot be owned by the function's Deployment and is never garbage
// collected. removeInterceptorRoute deletes it explicitly instead.
func generateInterceptorRoute(f fn.Function, namespace string, decorator deployer.DeployDecorator) (*unstructured.Unstructured, error) {
	labels, err := deployer.GenerateCommonLabels(f, decorator)
	if err != nil {
		return nil, err
	}
	// The name already encodes the function's namespace; the label makes it
	// selectable without parsing names apart.
	labels["function.knative.dev/namespace"] = namespace

	annotations := deployer.GenerateCommonAnnotations(f, decorator, false /* dapr n/a for routing */, KedaDeployerName)

	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]any{
				"name":        interceptorRouteName(f.Name, namespace),
				"namespace":   interceptorNamespace(),
				"labels":      stringMapToAny(labels),
				"annotations": stringMapToAny(annotations),
			},
			"spec": map[string]any{
				"to": map[string]any{
					"kind": "Service",
					"name": interceptorServiceName,
				},
				"port": map[string]any{
					"targetPort": interceptorServicePortName,
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

// stringMapToAny converts a map[string]string to the map[string]any that
// unstructured.Unstructured requires for nested fields.
func stringMapToAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// selectRouteURLs returns the URLs to report for a function: one per bridge
// host, cluster-internal on :8080. When routeFound, the Route's https URL is
// appended and returned as primary, since it is the reachable one.
func selectRouteURLs(hosts []string, routeHost string, routeFound bool) (primary string, all []string) {
	all = make([]string, 0, len(hosts)+1)
	for _, host := range hosts {
		// hosts carries the Route hostname as well, so the interceptor can
		// match it, but it is not a bridge address and must not get :8080.
		if routeFound && host == routeHost {
			continue
		}
		all = append(all, fmt.Sprintf("http://%s:8080", host))
	}
	if len(all) > 0 {
		primary = all[0]
	}
	if routeFound {
		routeURL := fmt.Sprintf("https://%s", routeHost)
		all = append(all, routeURL)
		primary = routeURL
	}
	return primary, all
}

// ensureInterceptorRoute creates or updates the shared-namespace Route
// exposing the interceptor for f, waits for it to be admitted, and returns
// the minted host. Reuses k8s.EnsureRoute/WaitForRouteAdmitted directly -
// both are already Route-shape-agnostic (they only need a namespace, name,
// and a pre-built unstructured object), so nothing keda-specific is needed
// there.
func ensureInterceptorRoute(ctx context.Context, dynClient dynamic.Interface, f fn.Function, namespace string, decorator deployer.DeployDecorator) (string, error) {
	route, err := generateInterceptorRoute(f, namespace, decorator)
	if err != nil {
		return "", fmt.Errorf("failed to generate interceptor Route: %w", err)
	}

	if err := k8s.EnsureRoute(ctx, dynClient, interceptorNamespace(), route); err != nil {
		return "", err
	}

	host, err := k8s.WaitForRouteAdmitted(ctx, dynClient, interceptorNamespace(), route.GetName(), 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("interceptor Route was not admitted: %w", err)
	}
	return host, nil
}

// removeInterceptorRoute deletes the interceptor Route for the function 'name'
// in 'namespace', but only if keda's deployer owns it: k8s.RemoveManagedRoute
// leaves a Route without the managed label in place.
//
// Every error is fatal, Forbidden included. Both callers run it LAST for that
// reason - remove() after the Deployment is already gone, Deploy() after the
// HTTPScaledObject exists - so a failure, typically no Route permissions in the
// interceptor namespace, orphans only the Route and never leaves the function
// half-removed or deployed without a scaler.
func removeInterceptorRoute(ctx context.Context, dynClient dynamic.Interface, name, namespace string) error {
	routeName := interceptorRouteName(name, namespace)
	if _, err := k8s.RemoveManagedRoute(ctx, dynClient, interceptorNamespace(), routeName, KedaDeployerName); err != nil {
		return fmt.Errorf("failed to remove interceptor Route %q in namespace %q: %w", routeName, interceptorNamespace(), err)
	}
	return nil
}
