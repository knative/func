package keda

import (
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	fn "knative.dev/func/pkg/functions"
)

func deployment(name, ns string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

// deploymentExists reports whether the named Deployment is still present.
func deploymentExists(t *testing.T, client *fake.Clientset, name, ns string) bool {
	t.Helper()
	_, err := client.AppsV1().Deployments(ns).Get(t.Context(), name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected error checking the Deployment: %v", err)
	}
	return err == nil
}

// Test_remove_RouteFailureStillDeletesDeployment asserts that when the Route
// removal fails, the Deployment is deleted anyway and the error still names the
// Route and its namespace.
func Test_remove_RouteFailureStillDeletesDeployment(t *testing.T) {
	clientset := fake.NewSimpleClientset(deployment("f", "fn-ns"))

	// Refuses every Route operation, as a user with no permissions in the
	// interceptor namespace would be refused.
	dynClient := newFakeDynamicClient()
	dynClient.PrependReactor("*", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "route.openshift.io", Resource: "routes"}, "f-fn-ns", errors.New("nope"))
	})

	err := remove(t.Context(), clientset, dynClient, "f", "fn-ns")

	if err == nil {
		t.Fatal("expected the Route failure to surface, so the user knows cleanup was incomplete")
	}
	if deploymentExists(t, clientset, "f", "fn-ns") {
		t.Error("the Deployment must be deleted even when the Route removal fails - it is the whole point of the ordering")
	}
	if !strings.Contains(err.Error(), "f-fn-ns") || !strings.Contains(err.Error(), interceptorNamespace()) {
		t.Errorf("expected the error to name the Route and its namespace, got %v", err)
	}
}

// Test_remove_Succeeds asserts the Deployment is deleted when removing the
// Route succeeds too, and that remove() really does remove the Route. The Route
// has to be seeded: RemoveManagedRoute treats a missing Route as success, so
// against an empty client this test would pass even if remove() dropped the
// Route call outright - which is the one path this rung reorders.
func Test_remove_Succeeds(t *testing.T) {
	clientset := fake.NewSimpleClientset(deployment("f", "fn-ns"))
	route, err := generateInterceptorRoute(fn.Function{Name: "f", Runtime: "go"}, "fn-ns", nil)
	if err != nil {
		t.Fatal(err)
	}
	dynClient := newFakeDynamicClient(route)

	if err := remove(t.Context(), clientset, dynClient, "f", "fn-ns"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deploymentExists(t, clientset, "f", "fn-ns") {
		t.Error("expected the Deployment to be deleted")
	}
	if _, err := dynClient.Resource(routeGVR).Namespace(interceptorNamespace()).
		Get(t.Context(), interceptorRouteName("f", "fn-ns"), metav1.GetOptions{}); err == nil {
		t.Error("expected remove() to delete the interceptor Route, not only the Deployment")
	}
}

// Test_remove_NoRouteAPI covers a non-OpenShift cluster, where Remove passes a
// nil dynClient because no Route can exist.
func Test_remove_NoRouteAPI(t *testing.T) {
	clientset := fake.NewSimpleClientset(deployment("f", "fn-ns"))

	if err := remove(t.Context(), clientset, nil, "f", "fn-ns"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deploymentExists(t, clientset, "f", "fn-ns") {
		t.Error("expected the Deployment to be deleted")
	}
}

// Test_remove_FunctionNotFound asserts a missing Deployment yields
// fn.ErrFunctionNotFound rather than any other error.
func Test_remove_FunctionNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // no Deployment
	dynClient := newFakeDynamicClient()

	err := remove(t.Context(), clientset, dynClient, "f", "fn-ns")
	if !errors.Is(err, fn.ErrFunctionNotFound) {
		t.Fatalf("expected fn.ErrFunctionNotFound, got %v", err)
	}
}
