package keda

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/ocproute"
)

const (
	testFnName        = "f"
	testFnNS          = "fn-keda"
	testInterceptorNS = interceptorNamespaceUpstream
	testHost          = "f-ns.apps.example.com"
)

// newTestClientset returns a clientset holding the function's Service;
// broken makes every Service update fail, standing in for a record that
// cannot be written or cleared.
func newTestClientset(broken bool, annotations map[string]string) *fake.Clientset {
	clientset := fake.NewClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: testFnName, Namespace: testFnNS, Annotations: annotations,
	}})
	if broken {
		clientset.PrependReactor("update", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("boom")
		})
	}
	return clientset
}

func newTestDynClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{testRouteGVR: "RouteList"},
		objects...)
}

func testRecord() map[string]string {
	return map[string]string{
		k8s.RouteHostnameAnnotation:  testHost,
		k8s.RouteNamespaceAnnotation: testInterceptorNS,
	}
}

// newTestTarget bundles the fakes the way Deploy bundles the real clients.
func newTestTarget(clientset *fake.Clientset, dynClient *dynamicfake.FakeDynamicClient) deployTarget {
	return deployTarget{
		clientset: clientset,
		dynClient: dynClient,
		ref:       deployer.NewExposureRef(testFnName, testFnNS, testInterceptorNS),
	}
}

func routeCount(t *testing.T, dynClient *dynamicfake.FakeDynamicClient) int {
	t.Helper()
	list, err := dynClient.Resource(testRouteGVR).Namespace(testInterceptorNS).List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

// Test_RecordExposure_kedaRollback: deployExposed records, then Unexposes
// if that write fails. Keda's Route has no owner reference, so the rollback
// is the only collector. These tests hit those two calls without an HSO
// client.
func Test_RecordExposure_kedaRollback(t *testing.T) {
	routeName := interceptorExposureName(testFnName, testFnNS)
	d := NewDeployer(WithExposer(ocproute.New(deployers.Keda)))

	t.Run("record failure rolls the Route back", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		target := newTestTarget(newTestClientset(true, nil), dynClient)

		err := k8s.RecordExposure(t.Context(), target.clientset, target.ref, testHost)
		if err == nil {
			t.Fatal("expected the record failure to be reported")
		}
		if rbErr := d.exposer.Unexpose(t.Context(), target.dynClient, target.ref); rbErr != nil {
			t.Fatalf("expected rollback to succeed, got: %v", rbErr)
		}
		if n := routeCount(t, dynClient); n != 0 {
			t.Errorf("expected the just-created Route rolled back, %d left", n)
		}
	})

	t.Run("record failure and failed rollback leave the Route", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		dynClient.PrependReactor("delete", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("delete denied")
		})
		target := newTestTarget(newTestClientset(true, nil), dynClient)

		err := k8s.RecordExposure(t.Context(), target.clientset, target.ref, testHost)
		rbErr := d.exposer.Unexpose(t.Context(), target.dynClient, target.ref)
		if err == nil || rbErr == nil {
			t.Fatalf("expected both failures, record=%v rollback=%v", err, rbErr)
		}
		if n := routeCount(t, dynClient); n != 1 {
			t.Errorf("expected the Route left when rollback fails, %d found", n)
		}
	})

	t.Run("a written record keeps the Route", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		clientset := newTestClientset(false, nil)
		target := newTestTarget(clientset, dynClient)

		if err := k8s.RecordExposure(t.Context(), target.clientset, target.ref, testHost); err != nil {
			t.Fatalf("expected the record to succeed, got: %v", err)
		}
		if n := routeCount(t, dynClient); n != 1 {
			t.Errorf("expected the Route kept, %d found", n)
		}
		svc, getErr := clientset.CoreV1().Services(testFnNS).Get(t.Context(), testFnName, metav1.GetOptions{})
		if getErr != nil {
			t.Fatal(getErr)
		}
		if svc.Annotations[k8s.RouteHostnameAnnotation] != testHost ||
			svc.Annotations[k8s.RouteNamespaceAnnotation] != testInterceptorNS {
			t.Errorf("expected both record annotations written, got: %v", svc.Annotations)
		}
	})
}

// Test_clearExposure: the cluster-local path's teardown. The Route is
// removed BEFORE the record is cleared, so a failed removal leaves the
// record for the retry to act on; a failed clear touches no Route at all,
// and a nil Exposer leaves everything alone.
func Test_clearExposure(t *testing.T) {
	routeName := interceptorExposureName(testFnName, testFnNS)
	d := NewDeployer(WithExposer(ocproute.New(deployers.Keda)))

	t.Run("removes the recorded Route, then clears the record", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		clientset := newTestClientset(false, testRecord())

		if err := d.clearExposure(t.Context(), newTestTarget(clientset, dynClient), testInterceptorNS); err != nil {
			t.Fatalf("expected the opt-out to succeed, got: %v", err)
		}
		if n := routeCount(t, dynClient); n != 0 {
			t.Errorf("expected the recorded Route removed, %d left", n)
		}
		svc, getErr := clientset.CoreV1().Services(testFnNS).Get(t.Context(), testFnName, metav1.GetOptions{})
		if getErr != nil {
			t.Fatal(getErr)
		}
		if _, ok := svc.Annotations[k8s.RouteHostnameAnnotation]; ok {
			t.Error("expected the hostname record cleared")
		}
		if _, ok := svc.Annotations[k8s.RouteNamespaceAnnotation]; ok {
			t.Error("expected the namespace record cleared")
		}
	})

	t.Run("a failed removal leaves the record for the retry", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		dynClient.PrependReactor("list", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("list denied")
		})
		clientset := newTestClientset(false, testRecord())

		err := d.clearExposure(t.Context(), newTestTarget(clientset, dynClient), testInterceptorNS)
		if err == nil || !strings.Contains(err.Error(), "failed to remove external exposure") {
			t.Fatalf("expected the removal failure reported, got: %v", err)
		}
		svc, getErr := clientset.CoreV1().Services(testFnNS).Get(t.Context(), testFnName, metav1.GetOptions{})
		if getErr != nil {
			t.Fatal(getErr)
		}
		if svc.Annotations[k8s.RouteNamespaceAnnotation] != testInterceptorNS {
			t.Error("expected the record left in place for the retry to act on")
		}
	})

	t.Run("a failed clear touches no Route", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		// A half record (hostname only, no namespace): nothing says a Route
		// exists, so nothing is removed; the clear write fails and must be
		// the only thing that happened.
		clientset := newTestClientset(true, map[string]string{
			k8s.RouteHostnameAnnotation: testHost,
		})

		err := d.clearExposure(t.Context(), newTestTarget(clientset, dynClient), "")
		if err == nil {
			t.Fatal("expected the failed clear to be reported")
		}
		if n := len(dynClient.Actions()); n != 0 {
			t.Errorf("a failed clear must not touch the Route API, saw %d calls", n)
		}
	})

	t.Run("nil exposer leaves Route and record alone", func(t *testing.T) {
		dynClient := newTestDynClient(kedaRoute(routeName, testInterceptorNS, testFnName, testFnNS))
		clientset := newTestClientset(false, testRecord())

		if err := NewDeployer().clearExposure(t.Context(), newTestTarget(clientset, dynClient), testInterceptorNS); err != nil {
			t.Fatalf("expected a nil exposer to be a no-op, got: %v", err)
		}
		if n := len(dynClient.Actions()); n != 0 {
			t.Errorf("a nil exposer must not touch the Route API, saw %d calls", n)
		}
		svc, getErr := clientset.CoreV1().Services(testFnNS).Get(t.Context(), testFnName, metav1.GetOptions{})
		if getErr != nil {
			t.Fatal(getErr)
		}
		if svc.Annotations[k8s.RouteNamespaceAnnotation] != testInterceptorNS {
			t.Error("expected the record left untouched: no Exposer means nothing here owns it")
		}
	})
}

// Test_validateExposure: the interceptor refusal is surfaced only when an
// exposure is actually wanted; a cluster-local deploy must not fail on an
// interceptor nobody asked to route through.
func Test_validateExposure(t *testing.T) {
	refusal := fmt.Errorf("interceptor missing")
	d := NewDeployer(WithExposer(ocproute.New(deployers.Keda)))

	f := fn.Function{Name: testFnName, Namespace: testFnNS, Expose: fn.ExposeRoute}
	if err := d.validateExposure(f, refusal); err == nil || !strings.Contains(err.Error(), "cannot expose") {
		t.Errorf("expected the refusal surfaced for an active intent, got: %v", err)
	}

	if err := d.validateExposure(f, nil); err != nil {
		t.Errorf("expected a valid active intent to pass, got: %v", err)
	}

	f.Expose = ""
	if err := d.validateExposure(f, refusal); err != nil {
		t.Errorf("expected no error when no exposure is wanted, got: %v", err)
	}

	f.Expose = fn.ExposeRoute
	if err := NewDeployer().validateExposure(f, refusal); err != nil {
		t.Errorf("expected a nil exposer to skip exposure validation, got: %v", err)
	}
}
