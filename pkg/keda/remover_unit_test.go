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

// Test_unexposeRecorded pins removal's whole contract: the record is acted on
// as written - one namespace, nothing guessed, nothing widened - and the
// stamp and label checks still decide what may be removed there. Exposure a
// crash left unrecorded is deliberately not hunted; the next exposed redeploy
// reconciles it away by label.
//
// Assertions are on the fake's object state and on returned refusals: what
// left the cluster and what was reported are all a caller can observe.
func Test_unexposeRecorded(t *testing.T) {
	const (
		recorded = interceptorNamespaceUpstream  // where the record says the Route is
		other    = interceptorNamespaceOpenShift // a namespace the record does not name
		fnName   = "f"
		fnNS     = "fn-keda"
	)
	routeName := interceptorExposureName(fnName, fnNS)
	x := ocproute.New(KedaDeployerName)

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

	// The record names the one namespace to act on; nowhere else is even
	// consulted - the other candidate answers Forbidden here precisely to
	// prove it is never asked.
	t.Run("removes the Route from the recorded namespace", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, recorded, fnName, fnNS))
		forbidIn(client, other)

		if err := x.Unexpose(t.Context(), client, deployer.NewExposureRef(fnName, fnNS, recorded)); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, recorded); n != 0 {
			t.Errorf("expected the Route to be gone, %d left in %q", n, recorded)
		}
	})

	// The record widens where removal looks, never what it may remove: the
	// stamp and label checks still decide, so another deployer's Route
	// survives even in the recorded namespace.
	t.Run("a Route with another deployer's stamp survives", func(t *testing.T) {
		stranger := kedaRoute(routeName, recorded, fnName, fnNS)
		if err := unstructured.SetNestedField(stranger.Object, deployers.Kubernetes,
			"metadata", "annotations", deployer.DeployerNameAnnotation); err != nil {
			t.Fatal(err)
		}
		client := newClient(stranger)

		if err := x.Unexpose(t.Context(), client, deployer.NewExposureRef(fnName, fnNS, recorded)); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, recorded); n != 1 {
			t.Errorf("a Route stamped by another deployer must survive, found %d in %q", n, recorded)
		}
	})

	// A record means a Route WAS made, so failing to take it away is a fact
	// the caller is owed. Remove reports it before the function is
	// dismantled, which is what makes a retry after the grant an ordinary
	// delete.
	t.Run("denial on the recorded namespace is fatal", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, recorded, fnName, fnNS))
		forbidIn(client, recorded)

		if err := x.Unexpose(t.Context(), client, deployer.NewExposureRef(fnName, fnNS, recorded)); err == nil {
			t.Fatal("expected a denial on a recorded Route to fail the removal")
		}
	})

	// A recorded namespace that holds no Route is a clean answer, not an
	// error, and nothing widens the search elsewhere afterwards.
	t.Run("a record naming an empty namespace removes nothing and does not widen", func(t *testing.T) {
		client := newClient(kedaRoute(routeName, other, fnName, fnNS))

		if err := x.Unexpose(t.Context(), client, deployer.NewExposureRef(fnName, fnNS, recorded)); err != nil {
			t.Fatalf("unexpected failure: %v", err)
		}
		if n := routesIn(t, client, other); n != 1 {
			t.Errorf("a Route outside the recorded namespace must be left alone, found %d in %q", n, other)
		}
	})
}
