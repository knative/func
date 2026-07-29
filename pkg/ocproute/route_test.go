package ocproute

import (
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stesting "k8s.io/client-go/testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
)

// errDenied stands in for the reason an API server gives with a 403.
var errDenied = errors.New("denied")

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

// testExposure is what the raw deployer asks for: the function's own Service,
// in the function's own namespace, owned by its Deployment.
func testExposure() deployer.Exposure {
	d := testDeployment()
	controller := true
	return deployer.Exposure{
		Function:          fn.Function{Name: "f", Runtime: "go"},
		FunctionNamespace: d.Namespace,
		Name:              d.Name,
		Namespace:         d.Namespace,
		TargetService:     d.Name,
		TargetPort:        "http",
		Owner: &metav1.OwnerReference{
			APIVersion: appsv1.SchemeGroupVersion.WithKind("Deployment").GroupVersion().String(),
			Kind:       "Deployment",
			Name:       d.Name,
			UID:        d.UID,
			Controller: &controller,
		},
	}
}

func testExposer() *Exposer {
	return New(deployers.Kubernetes)
}

func Test_generate(t *testing.T) {
	route, err := testExposer().generate(testExposure())
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
	if !testExposer().isManaged(route) {
		t.Error("expected a freshly generated Route to be self-managed")
	}
	owners := route.GetOwnerReferences()
	if len(owners) != 1 || owners[0].Name != "f" || owners[0].Kind != "Deployment" {
		t.Errorf("expected a single Deployment ownerRef named f, got %+v", owners)
	}
}

// Test_generate_Domain: a custom domain becomes spec.host verbatim; without
// one the field is absent and the router mints the hostname.
func Test_generate_Domain(t *testing.T) {
	e := testExposure()
	e.Function.Domain = "hello.tester1.com"
	route, err := testExposer().generate(e)
	if err != nil {
		t.Fatal(err)
	}
	if host, _, _ := unstructured.NestedString(route.Object, "spec", "host"); host != "hello.tester1.com" {
		t.Errorf("expected spec.host %q, got %q", "hello.tester1.com", host)
	}

	route, err = testExposer().generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	if host, found, _ := unstructured.NestedString(route.Object, "spec", "host"); found {
		t.Errorf("expected no spec.host without a domain, got %q", host)
	}
}

// Test_ensure_PreservesInjectedTLS: an update must carry over TLS material a
// certificate controller wrote into spec.tls (cert-manager's openshift-routes
// plugin injects a custom domain's cert there); regenerating the spec must
// not wipe it on redeploy.
func Test_ensure_PreservesInjectedTLS(t *testing.T) {
	ctx := t.Context()
	x := testExposer()

	existing, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(existing.Object, "PEM-CERT", "spec", "tls", "certificate"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(existing.Object, "PEM-KEY", "spec", "tls", "key"); err != nil {
		t.Fatal(err)
	}
	client := newFakeDynamicClient(existing)

	fresh, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.ensure(ctx, client, testExposure(), fresh); err != nil {
		t.Fatal(err)
	}

	got, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cert, _, _ := unstructured.NestedString(got.Object, "spec", "tls", "certificate"); cert != "PEM-CERT" {
		t.Errorf("expected the injected certificate carried over, got %q", cert)
	}
	if key, _, _ := unstructured.NestedString(got.Object, "spec", "tls", "key"); key != "PEM-KEY" {
		t.Errorf("expected the injected key carried over, got %q", key)
	}
	if term, _, _ := unstructured.NestedString(got.Object, "spec", "tls", "termination"); term != "edge" {
		t.Errorf("expected func's own tls fields still applied, got termination %q", term)
	}
}

// Test_ensure_DomainChangeRecreates: updating spec.host in place is gated on
// routes/custom-host update permission, so a changed domain replaces the
// Route instead of updating it; a stale injected cert dies with the old host.
func Test_ensure_DomainChangeRecreates(t *testing.T) {
	ctx := t.Context()
	x := testExposer()

	existing, err := x.generate(testExposure()) // no domain: router-minted host
	if err != nil {
		t.Fatal(err)
	}
	client := newFakeDynamicClient(existing)

	e := testExposure()
	e.Function.Domain = "hello.tester1.com"
	fresh, err := x.generate(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.ensure(ctx, client, e, fresh); err != nil {
		t.Fatal(err)
	}

	var deleted bool
	for _, a := range client.Actions() {
		if a.GetVerb() == "delete" {
			deleted = true
		}
	}
	if !deleted {
		t.Error("expected the old Route deleted, not updated: spec.host is immutable")
	}
	got, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if host, _, _ := unstructured.NestedString(got.Object, "spec", "host"); host != "hello.tester1.com" {
		t.Errorf("expected the recreated Route to carry spec.host %q, got %q", "hello.tester1.com", host)
	}
}

// Test_generate_NoOwner covers the keda-shaped Exposure: a Route in a
// namespace the function's Deployment does not live in, which Kubernetes
// forbids owning across, so it carries no ownerReference at all.
func Test_generate_NoOwner(t *testing.T) {
	e := testExposure()
	e.Owner = nil
	e.Namespace = "openshift-keda"
	e.Name = "f-ns"
	e.TargetService = "keda-add-ons-http-interceptor-proxy"
	e.TargetPort = "proxy"

	route, err := testExposer().generate(e)
	if err != nil {
		t.Fatal(err)
	}

	if owners := route.GetOwnerReferences(); len(owners) != 0 {
		t.Errorf("expected no ownerRef when Owner is nil, got %+v", owners)
	}
	if route.GetName() != "f-ns" || route.GetNamespace() != "openshift-keda" {
		t.Errorf("expected name/namespace f-ns/openshift-keda, got %s/%s", route.GetName(), route.GetNamespace())
	}
	toName, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name")
	if toName != "keda-add-ons-http-interceptor-proxy" {
		t.Errorf("expected spec.to.name to be the interceptor Service, got %q", toName)
	}
	targetPort, _, _ := unstructured.NestedString(route.Object, "spec", "port", "targetPort")
	if targetPort != "proxy" {
		t.Errorf("expected spec.port.targetPort proxy, got %q", targetPort)
	}
}

// Test_isManaged_ForeignDeployer: a Route stamped by one deployer is not
// managed by another's Exposer, so neither deletes the other's Route.
func Test_isManaged_ForeignDeployer(t *testing.T) {
	route, err := New(deployers.Keda).generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	if New(deployers.Kubernetes).isManaged(route) {
		t.Error("expected a keda-stamped Route not to be managed by the raw Exposer")
	}
	if !New(deployers.Keda).isManaged(route) {
		t.Error("expected a keda-stamped Route to be managed by the keda Exposer")
	}
}

func Test_ensure_CreateThenUpdate(t *testing.T) {
	ctx := t.Context()
	x := testExposer()
	client := newFakeDynamicClient()

	route, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	name, err := x.ensure(ctx, client, testExposure(), route)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if name != "f" {
		t.Errorf("expected the generated name f, got %q", name)
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
	route2, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.ensure(ctx, client, testExposure(), route2); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// Test_ensure_RefusesForeignRoute pins the half of the ownership rule that
// used to be missing. Unexpose has always declined to delete a Route func did
// not create; ensure used to fetch by name and update whatever it found, so
// the same object delete protected was silently taken over by create. Both
// paths now ask isManaged, and creation fails loudly on the name collision
// rather than overwriting somebody's object and reporting success.
func Test_ensure_RefusesForeignRoute(t *testing.T) {
	ctx := t.Context()
	foreign := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata": map[string]any{
			"name":      "f",
			"namespace": "ns",
		},
		"spec": map[string]any{
			"to": map[string]any{"kind": "Service", "name": "someone-elses-service"},
		},
	}}
	client := newFakeDynamicClient(foreign)

	x := testExposer()
	route, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := x.ensure(ctx, client, testExposure(), route); err == nil {
		t.Fatal("expected ensure to refuse a Route func did not create, got nil error")
	}

	got, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	toName, _, _ := unstructured.NestedString(got.Object, "spec", "to", "name")
	if toName != "someone-elses-service" {
		t.Errorf("the foreign Route was overwritten: spec.to.name is now %q", toName)
	}
}

// Test_find_ByLabelNotName is the point of selecting on labels: func's own
// Route is found even under a name func would not choose today, so changing
// the naming scheme cannot strand one, and a foreign Route sitting at the
// name func WOULD choose is not found at all.
func Test_find_ByLabelNotName(t *testing.T) {
	ctx := t.Context()
	x := testExposer()

	renamed, err := x.generate(testExposure())
	if err != nil {
		t.Fatal(err)
	}
	renamed.SetName("f-under-some-older-scheme")

	foreign := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata":   map[string]any{"name": "f", "namespace": "ns"},
	}}

	client := newFakeDynamicClient(renamed, foreign)

	found, err := x.find(ctx, client, testExposure().Ref())
	if err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatal("expected func's own Route to be found under its old name")
	}
	if found.GetName() != "f-under-some-older-scheme" {
		t.Errorf("found the wrong Route: %q", found.GetName())
	}
}

// Test_find_IgnoresAnotherFunction: the name label alone does not separate two
// functions called the same in different namespaces, which is the whole reason
// the namespace label exists. Keda puts every function's Route in one shared
// namespace, so without it a teardown would find a stranger's.
func Test_find_IgnoresAnotherFunction(t *testing.T) {
	ctx := t.Context()
	x := testExposer()

	other := testExposure()
	other.FunctionNamespace = "somebody-else"
	other.Name = "f-somebody-else"
	otherRoute, err := x.generate(other)
	if err != nil {
		t.Fatal(err)
	}
	// Both Routes land in one namespace, as keda's do.
	otherRoute.SetNamespace("ns")

	client := newFakeDynamicClient(otherRoute)

	found, err := x.find(ctx, client, testExposure().Ref())
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Errorf("found another function's Route %q", found.GetName())
	}
}

// Test_find_DeniedIsNotVisible: a denied lookup must not be reported as "no
// Route here". A denied account is told the same thing whether one exists or
// not, and callers differ on whether that should be fatal, so the two answers
// have to stay distinguishable.
func Test_find_DeniedIsNotVisible(t *testing.T) {
	client := newFakeDynamicClient()
	client.PrependReactor("list", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: routeGVR.Group, Resource: routeGVR.Resource}, "", errDenied)
	})

	route, err := testExposer().find(t.Context(), client, testExposure().Ref())
	if err == nil {
		t.Fatal("expected a denied lookup to report an error, got nil")
	}
	if route != nil {
		t.Error("expected no Route alongside the error")
	}
	if !errors.Is(err, deployer.ErrExposureNotVisible) {
		t.Errorf("expected ErrExposureNotVisible so callers can tell denial from absence, got %v", err)
	}
}

// Test_Unexpose_DeniedPropagates: Unexpose must not report "nothing to remove"
// when it simply could not look. Reporting removed=false with no error would
// let a caller conclude the Route is gone.
func Test_Unexpose_DeniedPropagates(t *testing.T) {
	client := newFakeDynamicClient()
	client.PrependReactor("list", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: routeGVR.Group, Resource: routeGVR.Resource}, "", errDenied)
	})

	err := testExposer().Unexpose(t.Context(), client, testExposure().Ref())
	if err == nil {
		t.Fatal("expected a denied Unexpose to report an error")
	}
	if !errors.Is(err, deployer.ErrExposureNotVisible) {
		t.Errorf("expected ErrExposureNotVisible, got %v", err)
	}
}

func Test_Unexpose(t *testing.T) {
	ctx := t.Context()

	t.Run("nothing to remove: no-op", func(t *testing.T) {
		client := newFakeDynamicClient()
		if err := testExposer().Unexpose(ctx, client, testExposure().Ref()); err != nil {
			t.Errorf("expected nil for an absent Route, got %v", err)
		}
	})

	t.Run("managed: deleted", func(t *testing.T) {
		x := testExposer()
		route, err := x.generate(testExposure())
		if err != nil {
			t.Fatal(err)
		}
		client := newFakeDynamicClient(route)

		if err := x.Unexpose(ctx, client, testExposure().Ref()); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{}); err == nil {
			t.Error("expected Route to be gone after removal")
		}
	})

	t.Run("foreign Route at the same name: kept", func(t *testing.T) {
		foreign := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata":   map[string]any{"name": "f", "namespace": "ns"},
		}}
		client := newFakeDynamicClient(foreign)

		if err := testExposer().Unexpose(ctx, client, testExposure().Ref()); err != nil {
			t.Fatalf("expected nil for a foreign Route, got %v", err)
		}
		if _, err := client.Resource(routeGVR).Namespace("ns").Get(ctx, "f", metav1.GetOptions{}); err != nil {
			t.Error("expected the foreign Route to be left in place")
		}
	})
}

func Test_waitForAdmitted(t *testing.T) {
	ctx := t.Context()

	t.Run("admitted: returns host", func(t *testing.T) {
		client := newFakeDynamicClient(admittedRoute("f-ns.apps.example.com"))
		host, err := waitForAdmitted(ctx, client, "ns", "f", time.Second)
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
		_, err := waitForAdmitted(ctx, client, "ns", "f", 5*time.Second)
		if err == nil {
			t.Fatal("expected an error for a rejected Route")
		}
	})

	t.Run("rejected by one shard, admitted by another: admission wins", func(t *testing.T) {
		// The rejection entry comes FIRST: list order must not decide.
		mixed := &unstructured.Unstructured{Object: map[string]any{
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
					map[string]any{
						"host": "f-ns.apps.example.com",
						"conditions": []any{
							map[string]any{"type": "Admitted", "status": "True"},
						},
					},
				},
			},
		}}
		client := newFakeDynamicClient(mixed)
		host, err := waitForAdmitted(ctx, client, "ns", "f", time.Second)
		if err != nil {
			t.Fatalf("a rejection by one shard must not override an admission by another: %v", err)
		}
		if host != "f-ns.apps.example.com" {
			t.Errorf("expected the admitting shard's host, got %q", host)
		}
	})

	t.Run("never admitted: times out cleanly", func(t *testing.T) {
		route, err := testExposer().generate(testExposure())
		if err != nil {
			t.Fatal(err)
		}
		client := newFakeDynamicClient(route)

		_, err = waitForAdmitted(ctx, client, "ns", "f", 100*time.Millisecond)
		if err == nil {
			t.Fatal("expected a timeout error when no router ever admits the route")
		}
	})
}

// Test_waitForAdmitted_EmptyHost: a router that sets Admitted=True without a
// hostname has not given a usable answer. Returning it would hand the caller
// a bare "https://" and record the function as externally exposed, so the
// wait must not succeed on it.
func Test_waitForAdmitted_EmptyHost(t *testing.T) {
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata":   map[string]any{"name": "f", "namespace": "ns"},
		"status": map[string]any{
			"ingress": []any{map[string]any{
				"host": "",
				"conditions": []any{map[string]any{
					"type": "Admitted", "status": "True",
				}},
			}},
		},
	}}

	host, err := waitForAdmitted(t.Context(), newFakeDynamicClient(route), "ns", "f", 2*time.Second)
	if err == nil {
		t.Fatalf("expected an error for an admitted Route with no hostname, got host %q", host)
	}
	if host != "" {
		t.Errorf("expected no host alongside the error, got %q", host)
	}
	if !strings.Contains(err.Error(), "no hostname") {
		t.Errorf("expected the error to name the cause, got %v", err)
	}
}

func admittedRoute(host string) *unstructured.Unstructured {
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
