package keda

import (
	"context"
	"fmt"
	"math"
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

// TestReplicaBounds_MaxBelowOne documents the precondition Deploy's
// maxScale < 1 guard depends on: ValidateScale rejects scale.max: 0 for
// deployer: keda, but Deploy is reachable without going through
// Function.Validate first (library callers, tests), so replicaBounds can
// still hand back a maxScale that would produce an invalid (< 1) HPA
// maxReplicas if Deploy didn't check it itself.
func TestReplicaBounds_MaxBelowOne(t *testing.T) {
	zero := int64(0)
	f := fn.Function{Scale: &fn.ScaleOptions{Max: &zero}}
	_, max := replicaBounds(f)
	if max >= 1 {
		t.Fatalf("expected replicaBounds to pass scale.max: 0 through unchecked, got max=%d", max)
	}
}

// TestReplicaBounds_MinAboveMax documents the precondition Deploy's
// minScale > maxScale guard depends on: replicaBounds itself doesn't
// enforce scale.min <= scale.max, so an inconsistent pair reaches Deploy
// unchecked for callers that bypass Function.Validate.
func TestReplicaBounds_MinAboveMax(t *testing.T) {
	min, max := int64(5), int64(3)
	f := fn.Function{Scale: &fn.ScaleOptions{Min: &min, Max: &max}}
	gotMin, gotMax := replicaBounds(f)
	if gotMin <= gotMax {
		t.Fatalf("expected replicaBounds to pass scale.min > scale.max through unchecked, got min=%d max=%d", gotMin, gotMax)
	}
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

// TestDeploy_KafkaSASLPreflight covers the Deploy preflight for direct callers
// that bypass Function.Validate: an inconsistent SASL/security config must be
// rejected before any cluster resources are created. Otherwise buildScaledObject
// -- which only emits "sasl" trigger metadata for a non-empty mechanism -- would
// produce a ScaledObject that connects without SASL while the function's own
// container is configured for it. Each case returns from the pure preflight
// before Deploy touches the cluster, so no fake clientset is needed.
func TestDeploy_KafkaSASLPreflight(t *testing.T) {
	kafkaTrigger := &fn.ScaleOptions{
		KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "kafka"}}},
	}
	base := func(k *fn.KafkaConfig) fn.Function {
		return fn.Function{Name: testFnName, Scale: kafkaTrigger, Run: fn.RunSpec{Kafka: k}}
	}

	tests := []struct {
		name    string
		kafka   *fn.KafkaConfig
		wantErr string
	}{
		{
			name: "SASL_SSL with empty mechanism",
			kafka: &fn.KafkaConfig{
				Brokers: "b:9092", Topic: "t", ConsumerGroup: "g",
				SecurityProtocol: "SASL_SSL",
				SASL:             &fn.KafkaSASL{User: "u", Password: "p"},
			},
			wantErr: "run.kafka.sasl.mechanism is required",
		},
		{
			name: "SASL block with non-SASL protocol",
			kafka: &fn.KafkaConfig{
				Brokers: "b:9092", Topic: "t", ConsumerGroup: "g",
				SecurityProtocol: "SSL",
				SASL:             &fn.KafkaSASL{Mechanism: "PLAIN", User: "u", Password: "p"},
			},
			wantErr: "run.kafka.sasl requires securityProtocol SASL_PLAINTEXT or SASL_SSL",
		},
		{
			name: "unrecognized mechanism",
			kafka: &fn.KafkaConfig{
				Brokers: "b:9092", Topic: "t", ConsumerGroup: "g",
				SecurityProtocol: "SASL_SSL",
				SASL:             &fn.KafkaSASL{Mechanism: "OAUTHBEARER", User: "u", Password: "p"},
			},
			wantErr: "run.kafka.sasl.mechanism must be one of",
		},
	}

	d := NewDeployer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.Deploy(context.Background(), base(tt.kafka))
			if err == nil {
				t.Fatalf("expected Deploy to reject %s, got nil error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
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

// TestDeploy_ScaleOverflowPreflight covers the Deploy preflight guard against
// scale.min/max values that do not fit int32: replicaBounds narrows them to
// int32, and 1<<32 would silently wrap to 0 (and then masquerade as a valid
// small value) without this check.
func TestDeploy_ScaleOverflowPreflight(t *testing.T) {
	overflow := int64(math.MaxInt32) + 1
	valid := int64(3)
	trigger := &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "kafka"}}}

	tests := []struct {
		name    string
		scale   *fn.ScaleOptions
		wantErr string
	}{
		{"max overflow", &fn.ScaleOptions{Max: &overflow, KEDA: trigger}, "scale.max"},
		{"min overflow", &fn.ScaleOptions{Min: &overflow, Max: &valid, KEDA: trigger}, "scale.min"},
	}

	d := NewDeployer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.Deploy(context.Background(), fn.Function{Name: testFnName, Scale: tt.scale})
			if err == nil {
				t.Fatalf("expected Deploy to reject %s, got nil error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "out of range") {
				t.Fatalf("error %q is not the expected out-of-range error", err.Error())
			}
		})
	}
}

// TestDeploy_ScaleValidationPreflight covers the shared-ValidateScale sweep the
// Deploy preflight runs for direct callers that bypass Function.Validate: the
// value-range checks the tailored guards above it don't duplicate
// (pollingInterval/cooldownPeriod bounds, per-trigger threshold bounds, and the
// scale.keda<->scale.kpa mutual exclusion). Each returns from the pure preflight
// before any cluster call, so no fake clientset is needed.
func TestDeploy_ScaleValidationPreflight(t *testing.T) {
	httpTrigger := func() []fn.KEDATrigger { return []fn.KEDATrigger{{Type: "http"}} }

	tests := []struct {
		name    string
		scale   *fn.ScaleOptions
		wantErr string
	}{
		{
			name:    "pollingInterval below minimum",
			scale:   &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{PollingInterval: ptr.Int32(0), Triggers: httpTrigger()}},
			wantErr: "scale.keda.pollingInterval must be >= 1",
		},
		{
			name:    "cooldownPeriod below minimum",
			scale:   &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{CooldownPeriod: ptr.Int32(0), Triggers: httpTrigger()}},
			wantErr: "scale.keda.cooldownPeriod must be >= 1",
		},
		{
			name:    "http targetValue below minimum",
			scale:   &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "http", TargetValue: ptr.Int64(0)}}}},
			wantErr: "targetValue must be >= 1",
		},
		{
			name: "scale.keda and scale.kpa mutually exclusive",
			scale: &fn.ScaleOptions{
				KEDA: &fn.KEDAScaleOptions{Triggers: httpTrigger()},
				KPA:  &fn.KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			wantErr: "mutually exclusive",
		},
	}

	d := NewDeployer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.Deploy(context.Background(), fn.Function{Name: testFnName, Scale: tt.scale})
			if err == nil {
				t.Fatalf("expected Deploy to reject %s, got nil error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
