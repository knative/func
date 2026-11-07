package knative

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/pkg/ptr"
	"knative.dev/serving/pkg/apis/autoscaling"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// templateWithAnnotations returns a minimal RevisionTemplateSpec (one container,
// as setServiceOptions requires) seeded with the given annotations.
func templateWithAnnotations(annotations map[string]string) *servingv1.RevisionTemplateSpec {
	return &servingv1.RevisionTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
		Spec: servingv1.RevisionSpec{
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{{}},
			},
		},
	}
}

// TestSetServiceOptions_KPAAnnotations covers the scale.kpa -> autoscaling
// annotation mapping in setServiceOptions, which is otherwise only exercised by
// the cluster-backed TestInt_Deploy_WithOptions.
func TestSetServiceOptions_KPAAnnotations(t *testing.T) {
	t.Run("kpa set adds min/max and metric/target/utilization annotations", func(t *testing.T) {
		tmpl := templateWithAnnotations(nil)
		scale := &fn.ScaleOptions{
			Min: ptr.Int64(1),
			Max: ptr.Int64(10),
			KPA: &fn.KPAScaleOptions{
				Metric:      ptr.String("concurrency"),
				Target:      ptr.Float64(50),
				Utilization: ptr.Float64(80),
			},
		}
		if err := setServiceOptions(tmpl, scale, fn.Options{}); err != nil {
			t.Fatal(err)
		}
		a := tmpl.Annotations
		wantPresent := map[string]string{
			autoscaling.MinScaleAnnotationKey:          "1",
			autoscaling.MaxScaleAnnotationKey:          "10",
			autoscaling.MetricAnnotationKey:            "concurrency",
			autoscaling.TargetAnnotationKey:            "50.000000",
			autoscaling.TargetUtilizationPercentageKey: "80.000000",
		}
		for k, want := range wantPresent {
			if got := a[k]; got != want {
				t.Errorf("annotation %q = %q, want %q", k, got, want)
			}
		}
	})

	t.Run("kpa nil removes metric/target/utilization but keeps min/max", func(t *testing.T) {
		// A revision that previously carried kpa annotations should have them
		// cleared when scale.kpa is no longer set.
		tmpl := templateWithAnnotations(map[string]string{
			autoscaling.MetricAnnotationKey:            "rps",
			autoscaling.TargetAnnotationKey:            "200.000000",
			autoscaling.TargetUtilizationPercentageKey: "70.000000",
		})
		scale := &fn.ScaleOptions{Min: ptr.Int64(2), Max: ptr.Int64(5)}
		if err := setServiceOptions(tmpl, scale, fn.Options{}); err != nil {
			t.Fatal(err)
		}
		a := tmpl.Annotations
		if a[autoscaling.MinScaleAnnotationKey] != "2" {
			t.Errorf("min-scale = %q, want 2", a[autoscaling.MinScaleAnnotationKey])
		}
		if a[autoscaling.MaxScaleAnnotationKey] != "5" {
			t.Errorf("max-scale = %q, want 5", a[autoscaling.MaxScaleAnnotationKey])
		}
		for _, k := range []string{
			autoscaling.MetricAnnotationKey,
			autoscaling.TargetAnnotationKey,
			autoscaling.TargetUtilizationPercentageKey,
		} {
			if _, ok := a[k]; ok {
				t.Errorf("annotation %q should have been removed, got %q", k, a[k])
			}
		}
	})

	t.Run("nil scale leaves autoscaling annotations untouched", func(t *testing.T) {
		// With scale nil the whole block is skipped: pre-existing autoscaling
		// annotations must survive (nothing added, nothing removed).
		tmpl := templateWithAnnotations(map[string]string{
			autoscaling.MinScaleAnnotationKey: "3",
			autoscaling.MetricAnnotationKey:   "concurrency",
		})
		if err := setServiceOptions(tmpl, nil, fn.Options{}); err != nil {
			t.Fatal(err)
		}
		a := tmpl.Annotations
		if a[autoscaling.MinScaleAnnotationKey] != "3" {
			t.Errorf("min-scale = %q, want 3 (untouched)", a[autoscaling.MinScaleAnnotationKey])
		}
		if a[autoscaling.MetricAnnotationKey] != "concurrency" {
			t.Errorf("metric = %q, want concurrency (untouched)", a[autoscaling.MetricAnnotationKey])
		}
	})
}
