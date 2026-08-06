package keda

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	fn "knative.dev/func/pkg/functions"
)

var routeGVR = schema.GroupVersionResource{
	Group: "route.openshift.io", Version: "v1", Resource: "routes",
}

func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{routeGVR: "RouteList"},
		objects...,
	)
}

func Test_interceptorRouteName(t *testing.T) {
	// Same function name, different namespaces must not collide - the
	// whole reason the name includes the namespace, since every keda
	// function's Route lives together in the one shared interceptor namespace.
	a := interceptorRouteName("f", "ns1")
	b := interceptorRouteName("f", "ns2")
	if a == b {
		t.Fatalf("expected distinct names for the same function name in different namespaces, got %q for both", a)
	}
	if a != "f-ns1" || b != "f-ns2" {
		t.Errorf("expected f-ns1/f-ns2, got %q/%q", a, b)
	}
}

func Test_generateInterceptorRoute(t *testing.T) {
	f := fn.Function{Name: "f", Runtime: "go"}

	route, err := generateInterceptorRoute(f, "ns", nil)
	if err != nil {
		t.Fatal(err)
	}

	if route.GetName() != "f-ns" {
		t.Errorf("expected name f-ns, got %q", route.GetName())
	}
	if route.GetNamespace() != interceptorNamespace() {
		t.Errorf("expected namespace %q, got %q", interceptorNamespace(), route.GetNamespace())
	}
	toName, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name")
	if toName != interceptorServiceName {
		t.Errorf("expected spec.to.name %q (the interceptor, not the function's own Service), got %q", interceptorServiceName, toName)
	}
	targetPort, _, _ := unstructured.NestedString(route.Object, "spec", "port", "targetPort")
	if targetPort != interceptorServicePortName {
		t.Errorf("expected spec.port.targetPort %q, got %q", interceptorServicePortName, targetPort)
	}
	if len(route.GetOwnerReferences()) != 0 {
		t.Errorf("expected no ownerReferences (cross-namespace GC is impossible), got %+v", route.GetOwnerReferences())
	}
	if route.GetLabels()["function.knative.dev/namespace"] != "ns" {
		t.Errorf("expected the function.knative.dev/namespace label to disambiguate the shared route namespace, got %q",
			route.GetLabels()["function.knative.dev/namespace"])
	}
}

func Test_ensureAndRemoveInterceptorRoute(t *testing.T) {
	ctx := t.Context()
	f := fn.Function{Name: "f", Runtime: "go"}

	// ensureInterceptorRoute's admitted-returns-host path is NOT
	// independently exercised here: EnsureRoute's update branch replaces
	// the whole object with the freshly-generated one (no status field),
	// so a fake dynamic client can't simulate "already admitted, then
	// ensured again" without wiping the very status being asserted on -
	// the same fake-client status-subresource limitation noted in
	// pkg/k8s/route_test.go's commit. WaitForRouteAdmitted's own
	// admitted/rejected/timeout behavior is already covered directly and
	// fast there; this package only re-proves the timeout path below,
	// since ensureInterceptorRoute wires that call with its own hardcoded
	// duration worth confirming is reachable.

	t.Run("remove deletes a managed route", func(t *testing.T) {
		route, err := generateInterceptorRoute(f, "ns", nil)
		if err != nil {
			t.Fatal(err)
		}
		client := newFakeDynamicClient(route)

		if err := removeInterceptorRoute(ctx, client, "f", "ns"); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Resource(routeGVR).Namespace(interceptorNamespace()).Get(ctx, "f-ns", metav1.GetOptions{}); err == nil {
			t.Error("expected the interceptor Route to be gone after removal")
		}
	})

	t.Run("remove is a no-op when nothing exists", func(t *testing.T) {
		client := newFakeDynamicClient()
		if err := removeInterceptorRoute(ctx, client, "f", "ns"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ensure never admitted times out cleanly", func(t *testing.T) {
		route, err := generateInterceptorRoute(f, "ns", nil)
		if err != nil {
			t.Fatal(err)
		}
		client := newFakeDynamicClient(route)

		// A parent context with its own short deadline cancels the poll
		// well before ensureInterceptorRoute's hardcoded 30s internal
		// timeout is reached - proves the timeout path is reachable
		// without actually waiting 30 real seconds.
		shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, err = ensureInterceptorRoute(shortCtx, client, f, "ns", nil)
		if err == nil {
			t.Fatal("expected an error when no router ever admits the route")
		}
	})
}

func Test_selectRouteURLs(t *testing.T) {
	bridgeHosts := []string{"f-interceptor-bridge.ns.svc", "f-interceptor-bridge"}

	t.Run("no Route: bridge host is primary, both bridge hosts listed", func(t *testing.T) {
		primary, all := selectRouteURLs(bridgeHosts, "", false)
		if primary != "http://f-interceptor-bridge.ns.svc:8080" {
			t.Errorf("expected the first bridge host as primary, got %q", primary)
		}
		if len(all) != 2 || all[0] != "http://f-interceptor-bridge.ns.svc:8080" || all[1] != "http://f-interceptor-bridge:8080" {
			t.Errorf("expected both bridge hosts with :8080, got %v", all)
		}
	})

	t.Run("Route found: its https URL is primary and appended, bridge hosts unchanged", func(t *testing.T) {
		// hosts here matches the real call site's shape (deployer.go appends
		// the Route host onto the same slice HTTPScaledObject.Spec.Hosts
		// ends up with) - the Route host itself must appear exactly once,
		// portless https, never also as a :8080 bridge entry.
		hostsWithRoute := append(append([]string{}, bridgeHosts...), "f-ns.apps.example.com")
		primary, all := selectRouteURLs(hostsWithRoute, "f-ns.apps.example.com", true)
		if primary != "https://f-ns.apps.example.com" {
			t.Errorf("expected the Route's https URL as primary, got %q", primary)
		}
		if len(all) != 3 || all[2] != "https://f-ns.apps.example.com" {
			t.Errorf("expected the Route URL appended after the two bridge hosts, got %v", all)
		}
		if all[0] != "http://f-interceptor-bridge.ns.svc:8080" || all[1] != "http://f-interceptor-bridge:8080" {
			t.Errorf("expected the bridge hosts unaffected by the Route being found, got %v", all)
		}
		for _, u := range all {
			if u == "http://f-ns.apps.example.com:8080" {
				t.Errorf("expected the Route host NOT to also appear as a :8080 bridge entry, got %v", all)
			}
		}
	})

	t.Run("no hosts, no Route: empty primary, empty list", func(t *testing.T) {
		primary, all := selectRouteURLs(nil, "", false)
		if primary != "" || len(all) != 0 {
			t.Errorf("expected (\"\", empty), got (%q, %v)", primary, all)
		}
	})
}
