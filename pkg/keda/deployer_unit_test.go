package keda

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	"knative.dev/pkg/ptr"
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

func Test_replicaBounds(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }

	t.Run("defaults when scale is unset", func(t *testing.T) {
		min, max, err := replicaBounds(fn.Function{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if min != defaultMinReplicas || max != defaultMaxReplicas {
			t.Errorf("got min=%d max=%d, want %d/%d", min, max, defaultMinReplicas, defaultMaxReplicas)
		}
	})

	t.Run("valid values are used", func(t *testing.T) {
		min, max, err := replicaBounds(fn.Function{Scale: &fn.ScaleOptions{Min: i64(2), Max: i64(20)}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if min != 2 || max != 20 {
			t.Errorf("got min=%d max=%d, want 2/20", min, max)
		}
	})

	// ValidateScale only compares min and max when both are set explicitly, so
	// a min above keda's default max (max unset) reaches replicaBounds; it must
	// reject that rather than emit an HTTPScaledObject with min > max.
	t.Run("min above the effective default max errors", func(t *testing.T) {
		if _, _, err := replicaBounds(fn.Function{Scale: &fn.ScaleOptions{Min: i64(int64(defaultMaxReplicas) + 5)}}); err == nil {
			t.Error("expected an error when scale.min exceeds the defaulted scale.max, got nil")
		}
	})

	// keda maps scale.max to the HPA maxReplicas, which must be >= 1. An
	// explicit max: 0 is in range and passes the min > max check (0 > 0 is
	// false), so replicaBounds must reject it on its own for library callers
	// that bypass ValidateScale.
	t.Run("explicit max of zero errors", func(t *testing.T) {
		if _, _, err := replicaBounds(fn.Function{Scale: &fn.ScaleOptions{Min: i64(0), Max: i64(0)}}); err == nil {
			t.Error("expected an error for scale.max: 0, got nil")
		}
	})
}

// TestPollingIntervalIgnored covers the predicate that gates Deploy's
// "pollingInterval is ignored for http triggers" warning: only a kafka
// trigger's ScaledObject honors scale.keda.pollingInterval, so it must warn
// only for an http-only trigger that actually sets the value.
func TestPollingIntervalIgnored(t *testing.T) {
	interval := ptr.Int32(30)
	withInterval := &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{PollingInterval: interval}}
	withoutInterval := &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{}}

	tests := []struct {
		name                string
		scale               *fn.ScaleOptions
		wantHTTP, wantKafka bool
		want                bool
	}{
		{"http-only with pollingInterval warns", withInterval, true, false, true},
		{"http-only without pollingInterval is silent", withoutInterval, true, false, false},
		{"kafka trigger honors pollingInterval, no warning", withInterval, false, true, false},
		{"nil scale is silent", nil, true, false, false},
		{"nil KEDA is silent", &fn.ScaleOptions{}, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fn.Function{Name: testFnName, Scale: tt.scale}
			if got := pollingIntervalIgnored(f, tt.wantHTTP, tt.wantKafka); got != tt.want {
				t.Errorf("pollingIntervalIgnored() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDeploy_KafkaResourceNamePreflight covers the Deploy preflight guard for a
// function whose name is too long for the Kafka scaler resources: the
// ScaledObject (<name>-kafka) and TriggerAuthentication (<name>-kafka-auth)
// suffixes can overflow the 63-character DNS label limit, which would otherwise
// only surface as a server-side rejection mid-deploy. The check returns from the
// pure preflight before any cluster call.
func TestDeploy_KafkaResourceNamePreflight(t *testing.T) {
	kafkaTrigger := &fn.ScaleOptions{
		KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "kafka"}}},
	}
	// 58 chars: 58 + len("-kafka") == 64 > 63, so the ScaledObject name alone
	// overflows even without credentials configured.
	tooLong := strings.Repeat("a", 58)

	d := NewDeployer()
	_, err := d.Deploy(context.Background(), fn.Function{Name: tooLong, Scale: kafkaTrigger})
	if err == nil {
		t.Fatal("expected Deploy to reject an over-long function name, got nil error")
	}
	if !strings.Contains(err.Error(), "too long for the keda deployer") {
		t.Fatalf("error %q does not mention the name-length limit", err.Error())
	}
}

// Test_deleteTriggerAuthIfExists covers the in-type credential-drop cleanup:
// it deletes an existing TriggerAuthentication and issues no delete when none
// is present (so a kafka deploy that never had credentials never hits Forbidden
// on tighter RBAC).
func Test_deleteTriggerAuthIfExists(t *testing.T) {
	taName := triggerAuthName(testFnName)

	t.Run("deletes an existing TriggerAuthentication", func(t *testing.T) {
		dynClient := newScalingDynClient(unstructuredTriggerAuth(taName, testFnNS))
		deleteTriggerAuthIfExists(t.Context(), dynClient, testFnNS, testFnName)
		if _, err := dynClient.Resource(triggerAuthGVR).Namespace(testFnNS).Get(t.Context(), taName, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
			t.Errorf("expected TriggerAuthentication gone, got err: %v", err)
		}
	})

	t.Run("no TriggerAuthentication issues no delete", func(t *testing.T) {
		dynClient := newScalingDynClient()
		var deletes int
		dynClient.PrependReactor("delete", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
			deletes++
			return false, nil, nil
		})
		deleteTriggerAuthIfExists(t.Context(), dynClient, testFnNS, testFnName)
		if deletes != 0 {
			t.Fatalf("expected no delete calls when nothing exists, got %d", deletes)
		}
	})
}
