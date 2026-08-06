package k8s

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	fn "knative.dev/func/pkg/functions"
)

func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{routeGVR: "RouteList"},
		objects...,
	)
}

func testDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "f",
			Namespace: "ns",
			UID:       types.UID("abc-123"),
		},
	}
}

func Test_GenerateRoute(t *testing.T) {
	f := fn.Function{Name: "f", Runtime: "go"}
	deployment := testDeployment()

	route, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}

	if route.GetName() != "f" || route.GetNamespace() != "ns" {
		t.Errorf("expected name/namespace f/ns, got %s/%s", route.GetName(), route.GetNamespace())
	}
	toName, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name")
	if toName != "f" {
		t.Errorf("expected spec.to.name %q, got %q", "f", toName)
	}
	toKind, _, _ := unstructured.NestedString(route.Object, "spec", "to", "kind")
	if toKind != "Service" {
		t.Errorf("expected spec.to.kind Service, got %q", toKind)
	}
	targetPort, _, _ := unstructured.NestedString(route.Object, "spec", "port", "targetPort")
	if targetPort != "http" {
		t.Errorf("expected spec.port.targetPort http, got %q", targetPort)
	}
	if host, found, _ := unstructured.NestedString(route.Object, "spec", "host"); found && host != "" {
		t.Errorf("expected spec.host to be unset (router-minted), got %q", host)
	}
	termination, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination")
	if termination != "edge" {
		t.Errorf("expected spec.tls.termination edge, got %q", termination)
	}
	insecurePolicy, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "insecureEdgeTerminationPolicy")
	if insecurePolicy != "Redirect" {
		t.Errorf("expected spec.tls.insecureEdgeTerminationPolicy Redirect, got %q", insecurePolicy)
	}
	if !isManagedRoute(route, KubernetesDeployerName) {
		t.Error("expected a freshly generated Route to be self-managed")
	}
	owners := route.GetOwnerReferences()
	if len(owners) != 1 || owners[0].Name != "f" || owners[0].Kind != "Deployment" {
		t.Errorf("expected a single Deployment ownerRef named f, got %+v", owners)
	}
}

func Test_EnsureRoute_CreateThenUpdate(t *testing.T) {
	ctx := t.Context()
	f := fn.Function{Name: "f", Runtime: "go"}
	deployment := testDeployment()
	client := newFakeDynamicClient()

	route, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureRoute(ctx, client, "ns", route); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Route to exist after create: %v", err)
	}
	toName, _, _ := unstructured.NestedString(got.Object, "spec", "to", "name")
	if toName != "f" {
		t.Errorf("expected spec.to.name f, got %q", toName)
	}

	// Update path: regenerate (idempotent) and ensure again, no error.
	route2, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureRoute(ctx, client, "ns", route2); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func Test_RemoveManagedRoute(t *testing.T) {
	ctx := t.Context()

	t.Run("not found: no-op", func(t *testing.T) {
		client := newFakeDynamicClient()
		removed, err := RemoveManagedRoute(ctx, client, "ns", "f", KubernetesDeployerName)
		if err != nil || removed {
			t.Errorf("expected (false, nil), got (%v, %v)", removed, err)
		}
	})

	t.Run("managed: deleted", func(t *testing.T) {
		f := fn.Function{Name: "f", Runtime: "go"}
		deployment := testDeployment()
		route, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
		if err != nil {
			t.Fatal(err)
		}
		route.SetNamespace("ns")
		client := newFakeDynamicClient(route)

		removed, err := RemoveManagedRoute(ctx, client, "ns", "f", KubernetesDeployerName)
		if err != nil || !removed {
			t.Fatalf("expected (true, nil), got (%v, %v)", removed, err)
		}
		if _, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{}); err == nil {
			t.Error("expected Route to be gone after removal")
		}
	})

	t.Run("not managed: kept", func(t *testing.T) {
		foreign := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]any{
				"name":      "f",
				"namespace": "ns",
			},
		}}
		client := newFakeDynamicClient(foreign)

		removed, err := RemoveManagedRoute(ctx, client, "ns", "f", KubernetesDeployerName)
		if err != nil || removed {
			t.Fatalf("expected (false, nil) for a foreign Route, got (%v, %v)", removed, err)
		}
		if _, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{}); err != nil {
			t.Error("expected the foreign Route to be left in place")
		}
	})
}

func Test_WaitForRouteAdmitted(t *testing.T) {
	ctx := t.Context()

	admittedRoute := func(host string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata":   map[string]any{"name": "f", "namespace": "ns"},
			"status": map[string]any{
				"ingress": []any{
					map[string]any{
						"host": host,
						"conditions": []any{
							map[string]any{"type": "Admitted", "status": "True"},
						},
					},
				},
			},
		}}
	}

	t.Run("admitted: returns host", func(t *testing.T) {
		client := newFakeDynamicClient(admittedRoute("f-ns.apps.example.com"))
		host, err := WaitForRouteAdmitted(ctx, client, "ns", "f", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if host != "f-ns.apps.example.com" {
			t.Errorf("expected host f-ns.apps.example.com, got %q", host)
		}
	})

	t.Run("rejected: fails fast with reason", func(t *testing.T) {
		rejected := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata":   map[string]any{"name": "f", "namespace": "ns"},
			"status": map[string]any{
				"ingress": []any{
					map[string]any{
						"host": "",
						"conditions": []any{
							map[string]any{"type": "Admitted", "status": "False", "reason": "HostAlreadyClaimed", "message": "taken"},
						},
					},
				},
			},
		}}
		client := newFakeDynamicClient(rejected)
		_, err := WaitForRouteAdmitted(ctx, client, "ns", "f", 5*time.Second)
		if err == nil {
			t.Fatal("expected an error for a rejected Route")
		}
	})

	t.Run("never admitted: times out cleanly", func(t *testing.T) {
		f := fn.Function{Name: "f", Runtime: "go"}
		deployment := testDeployment()
		route, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
		if err != nil {
			t.Fatal(err)
		}
		route.SetNamespace("ns")
		client := newFakeDynamicClient(route)

		_, err = WaitForRouteAdmitted(ctx, client, "ns", "f", 100*time.Millisecond)
		if err == nil {
			t.Fatal("expected a timeout error when no router ever admits the route")
		}
	})
}

func Test_GetAdmittedRouteHost(t *testing.T) {
	ctx := t.Context()

	admittedRoute := func(host string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata":   map[string]any{"name": "f", "namespace": "ns"},
			"status": map[string]any{
				"ingress": []any{
					map[string]any{
						"host": host,
						"conditions": []any{
							map[string]any{"type": "Admitted", "status": "True"},
						},
					},
				},
			},
		}}
	}

	t.Run("admitted: returns host", func(t *testing.T) {
		client := newFakeDynamicClient(admittedRoute("f-ns.apps.example.com"))
		host, ok, err := GetAdmittedRouteHost(ctx, client, "ns", "f")
		if err != nil {
			t.Fatal(err)
		}
		if !ok || host != "f-ns.apps.example.com" {
			t.Errorf("expected (f-ns.apps.example.com, true), got (%q, %v)", host, ok)
		}
	})

	t.Run("not found: no error, not found", func(t *testing.T) {
		client := newFakeDynamicClient()
		host, ok, err := GetAdmittedRouteHost(ctx, client, "ns", "f")
		if err != nil || ok || host != "" {
			t.Errorf("expected (\"\", false, nil), got (%q, %v, %v)", host, ok, err)
		}
	})

	t.Run("not yet admitted: no error, not found", func(t *testing.T) {
		f := fn.Function{Name: "f", Runtime: "go"}
		deployment := testDeployment()
		route, err := GenerateRoute(f, "f", deployment, nil, KubernetesDeployerName)
		if err != nil {
			t.Fatal(err)
		}
		route.SetNamespace("ns")
		client := newFakeDynamicClient(route)

		host, ok, err := GetAdmittedRouteHost(ctx, client, "ns", "f")
		if err != nil || ok || host != "" {
			t.Errorf("expected (\"\", false, nil) for an unadmitted route, got (%q, %v, %v)", host, ok, err)
		}
	})
}
