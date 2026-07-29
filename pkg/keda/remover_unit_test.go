package keda

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fnlabels "knative.dev/func/pkg/k8s/labels"
	"knative.dev/func/pkg/ocproute"
)

var testRouteGVR = schema.GroupVersionResource{
	Group: "route.openshift.io", Version: "v1", Resource: "routes",
}

// kedaRoute is a Route as keda's Exposer stamps one: labelled with the
// function's name AND namespace, since every keda function's Route shares the
// interceptor's namespace, and annotated with the deployer that owns it.
func kedaRoute(routeName, routeNS, fnName, fnNS string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata": map[string]any{
			"name":      routeName,
			"namespace": routeNS,
			"labels": map[string]any{
				fnlabels.FunctionKey:          "true",
				fnlabels.FunctionNameKey:      fnName,
				fnlabels.FunctionNamespaceKey: fnNS,
			},
			"annotations": map[string]any{
				deployer.DeployerNameAnnotation: deployers.Keda,
			},
		},
	}}
}

// forbidIn makes every route verb in ns answer Forbidden, which is what a
// namespace the account cannot list answers - AND what a namespace that does
// not exist answers, since RBAC is evaluated before existence.
func forbidIn(client *dynamicfake.FakeDynamicClient, ns string) {
	client.PrependReactor("*", "routes", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() != ns {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: testRouteGVR.Group, Resource: testRouteGVR.Resource}, "", nil)
	})
}

// Test_unexpose pins the whole of what a removal decides about exposure: a
// record is removed as written, no record means sweeping the candidates.
//
// Assertions are on the FAKE'S OBJECT STATE and on whether the Route API was
// called at all, never on a return value: what left the cluster, and what was
// touched to find out, are the only things a caller can observe.
func Test_unexpose(t *testing.T) {
	const (
		recorded = interceptorNamespaceUpstream  // where the Route actually is
		other    = interceptorNamespaceOpenShift // the platform convention here
		fnName   = "f"
		fnNS     = "fn-keda"
	)
	routeName := interceptorExposureName(fnName, fnNS)

	newClient := func(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{testRouteGVR: "RouteList"},
			objects...)
	}
	routesIn := func(t *testing.T, c *dynamicfake.FakeDynamicClient, ns string) int {
		t.Helper()
		list, err := c.Resource(testRouteGVR).Namespace(ns).List(t.Context(), metav1.ListOptions{})
		if err != nil {
			t.Fatalf("counting Routes in %q: %v", ns, err)
		}
		return len(list.Items)
	}

	remover := NewRemover(false, WithRemoverExposer(ocproute.New(deployers.Keda)))

	// THE POINT OF THE CHANGE. The Route is in the namespace the deploy
	// recorded, which is NOT the platform convention. A removal that guessed
	// would look in the wrong place and report nothing to remove; this one is
	// told, so it removes it.
	t.Run("removes the Route from the recorded namespace", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, recorded, fnName, fnNS))
		// The namespace a guess would have chosen is unreadable, proving the
		// removal never goes there.
		forbidIn(client, other)

		if err := remover.unexpose(t.Context(), client, recorded, fnName, fnNS); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, recorded); n != 0 {
			t.Errorf("expected the Route to be gone, %d left in %q", n, recorded)
		}
	})

	// NO RECORD SWEEPS, on delete only. Deploy can stay silent because the next
	// deploy reuses the same deterministic name, but delete is the last moment
	// this function's identity exists and keda's Route carries no owner
	// reference to collect it afterwards. A stray left by a crash between
	// create and annotate is therefore hunted in every candidate namespace.
	t.Run("no record sweeps a stray from either candidate", func(t *testing.T) {
		for _, stray := range []string{recorded, other} {
			t.Run(stray, func(t *testing.T) {
				client := newClient(kedaRoute(routeName, stray, fnName, fnNS))

				if err := remover.unexpose(t.Context(), client, "", fnName, fnNS); err != nil {
					t.Fatalf("unexpected failure: %v", err)
				}
				if n := routesIn(t, client, stray); n != 0 {
					t.Errorf("expected the stray Route in %q to be swept, %d left", stray, n)
				}
			})
		}
	})

	// The ordinary case: nothing was ever exposed. Finding nothing is an
	// answer, not a failure.
	t.Run("no record and no Route anywhere is clean silence", func(t *testing.T) {
		client := newClient()

		if err := remover.unexpose(t.Context(), client, "", fnName, fnNS); err != nil {
			t.Fatalf("a sweep that finds nothing must not fail: %v", err)
		}
	})

	// A candidate that could not be listed is not a candidate that is empty.
	// Reporting a clean delete here would leave a Route serving with nothing
	// left to name it.
	t.Run("a candidate that cannot be listed is never read as empty", func(t *testing.T) {
		client := newClient()
		forbidIn(client, other)

		if err := remover.unexpose(t.Context(), client, "", fnName, fnNS); err == nil {
			t.Fatal("expected a denied candidate to fail the sweep, not report a clean delete")
		}
	})

	// The sweep widens WHERE it looks, never WHAT it will remove: the stamp
	// and label checks still decide, so another deployer's Route survives it.
	t.Run("a Route with another deployer's stamp survives the sweep", func(t *testing.T) {
		stranger := kedaRoute(routeName, recorded, fnName, fnNS)
		if err := unstructured.SetNestedField(stranger.Object, deployers.Kubernetes,
			"metadata", "annotations", deployer.DeployerNameAnnotation); err != nil {
			t.Fatal(err)
		}
		client := newClient(stranger)

		if err := remover.unexpose(t.Context(), client, "", fnName, fnNS); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, recorded); n != 1 {
			t.Errorf("a Route stamped by another deployer must survive, found %d in %q", n, recorded)
		}
	})

	// A record means a Route WAS made, so failing to take it away is a fact the
	// caller is owed. It is reported before the function is dismantled, which
	// is what makes a retry after the grant an ordinary delete.
	t.Run("denial on the recorded namespace is fatal", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, recorded, fnName, fnNS))
		forbidIn(client, recorded)

		if err := remover.unexpose(t.Context(), client, recorded, fnName, fnNS); err == nil {
			t.Fatal("expected a denial on a recorded Route to fail the removal")
		}
		// The Route's survival is not assertable here: the reactor forbids
		// listing in that namespace too, which is the whole point of the case.
		// The refusal IS the observable.
	})

	// The record is acted on as written. A recorded namespace that holds no
	// Route is a clean answer, not an error, and nothing widens the search to
	// the other candidate afterwards.
	t.Run("a record naming an empty namespace removes nothing and does not widen", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, other, fnName, fnNS))

		if err := remover.unexpose(t.Context(), client, recorded, fnName, fnNS); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, other); n != 1 {
			t.Errorf("a Route outside the recorded namespace must be left alone, found %d in %q", n, other)
		}
	})
}
