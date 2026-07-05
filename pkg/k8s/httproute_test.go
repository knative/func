package k8s

import (
	"context"
	goerrors "errors"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ktesting "k8s.io/client-go/testing"
	fn "knative.dev/func/pkg/functions"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestGenerateHTTPRoute_Basic(t *testing.T) {
	f := fn.Function{Name: "hello"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
	}

	route, err := GenerateHTTPRoute(f, "hello", 80, "hello.funcs.example.com", deployment, newGateway("func-gateway", "default", false), nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}

	if route.Name != "hello" {
		t.Errorf("expected name hello, got %q", route.Name)
	}
	if route.Namespace != "default" {
		t.Errorf("expected namespace default, got %q", route.Namespace)
	}

	if len(route.Spec.Hostnames) != 1 || string(route.Spec.Hostnames[0]) != "hello.funcs.example.com" {
		t.Errorf("expected hostname [hello.funcs.example.com], got %v", route.Spec.Hostnames)
	}

	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(route.Spec.ParentRefs))
	}
	ref := route.Spec.ParentRefs[0]
	if string(ref.Name) != "func-gateway" {
		t.Errorf("expected parentRef name func-gateway, got %v", ref.Name)
	}
	if ref.Namespace != nil {
		t.Errorf("parentRef should not have namespace when Gateway is in same namespace, got %v", *ref.Namespace)
	}

	ownerRefs := route.GetOwnerReferences()
	if len(ownerRefs) != 1 {
		t.Fatalf("expected 1 ownerRef, got %d", len(ownerRefs))
	}
	if ownerRefs[0].Name != "hello" {
		t.Errorf("expected ownerRef name hello, got %q", ownerRefs[0].Name)
	}
	if ownerRefs[0].Kind != "Deployment" {
		t.Errorf("expected ownerRef kind Deployment, got %q", ownerRefs[0].Kind)
	}

	if len(route.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(route.Spec.Rules))
	}
	rule := route.Spec.Rules[0]
	if len(rule.BackendRefs) != 1 {
		t.Fatalf("expected 1 backendRef, got %d", len(rule.BackendRefs))
	}
	backend := rule.BackendRefs[0]
	if string(backend.Name) != "hello" {
		t.Errorf("expected backendRef name hello, got %v", backend.Name)
	}
	if backend.Port == nil || *backend.Port != 80 {
		t.Errorf("expected backendRef port 80, got %v", backend.Port)
	}

	if route.Labels == nil {
		t.Fatal("expected labels to be set")
	}
	if route.Labels["boson.dev/function"] != "true" {
		t.Errorf("expected boson.dev/function=true label, got %v", route.Labels)
	}
}

func TestGenerateHTTPRoute_CrossNamespaceGateway(t *testing.T) {
	f := fn.Function{Name: "hello"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
	}

	route, err := GenerateHTTPRoute(f, "hello", 80, "hello.example.com", deployment, newGateway("shared-gateway", "infra", false), nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}

	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(route.Spec.ParentRefs))
	}
	ref := route.Spec.ParentRefs[0]
	if string(ref.Name) != "shared-gateway" {
		t.Errorf("expected parentRef name shared-gateway, got %v", ref.Name)
	}
	if ref.Namespace == nil || string(*ref.Namespace) != "infra" {
		t.Errorf("expected parentRef namespace infra, got %v", ref.Namespace)
	}
}

func TestGenerateHTTPRoute_CommonAnnotations(t *testing.T) {
	f := fn.Function{
		Name: "hello",
		Deploy: fn.DeploySpec{
			Annotations: map[string]string{"custom.example.com/foo": "bar"},
		},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
	}

	route, err := GenerateHTTPRoute(f, "hello", 80, "hello.example.com", deployment, newGateway("gw", "default", false), nil, KubernetesDeployerName)
	if err != nil {
		t.Fatal(err)
	}

	if route.Annotations["function.knative.dev/deployer"] != KubernetesDeployerName {
		t.Errorf("expected deployer annotation %q, got %v", KubernetesDeployerName, route.Annotations)
	}
	if route.Annotations["custom.example.com/foo"] != "bar" {
		t.Errorf("expected user annotation to be propagated, got %v", route.Annotations)
	}
}

// TestEnsureHTTPRoute_ClearsStaleResourceVersionOnCreateRetry proves that a
// first iteration that finds the route (Update) but hits a Conflict must
// not leak its ResourceVersion into a second iteration's Create after the
// route disappeared out from under it - the apiserver rejects a Create that
// carries one. Reactor sequence: Get(found)->Update(Conflict)->Get(NotFound)->Create.
func TestEnsureHTTPRoute_ClearsStaleResourceVersionOnCreateRetry(t *testing.T) {
	ctx := context.Background()
	gwFake := newGwFake()

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default"},
	}

	gr := schema.GroupResource{Group: gwv1.GroupName, Resource: "httproutes"}

	getCalls := 0
	gwFake.PrependReactor("get", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
		getCalls++
		if getCalls == 1 {
			// First iteration: the route exists, so EnsureHTTPRoute() takes the
			// Update path and stamps route.ResourceVersion from this object.
			return true, &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default", ResourceVersion: "1"},
			}, nil
		}
		// Second iteration: the route is gone (e.g. deleted concurrently) -
		// EnsureHTTPRoute() must fall back to Create().
		return true, nil, apierrors.NewNotFound(gr, "myfunc")
	})
	gwFake.PrependReactor("update", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(gr, "myfunc", goerrors.New("stale resourceVersion"))
	})
	gwFake.PrependReactor("create", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
		created := action.(ktesting.CreateAction).GetObject().(*gwv1.HTTPRoute)
		if created.ResourceVersion != "" {
			return true, nil, fmt.Errorf("Create must not carry a resourceVersion, got %q", created.ResourceVersion)
		}
		return false, nil, nil // fall through to the default tracker to actually store it
	})

	if err := EnsureHTTPRoute(ctx, gwFake, "default", route); err != nil {
		t.Fatalf("expected EnsureHTTPRoute to succeed via the Create fallback, got: %v", err)
	}
}
