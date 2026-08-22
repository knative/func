package keda

import (
	"testing"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	fn "knative.dev/func/pkg/functions"
)

func TestHTTPScaledObjectDefaultsMinReplicasToZero(t *testing.T) {
	httpScaledObject := newTestHTTPScaledObject(t, fn.Function{})

	if httpScaledObject.Spec.Replicas == nil || httpScaledObject.Spec.Replicas.Min == nil {
		t.Fatal("expected replicas.min to be set")
	}

	if got := *httpScaledObject.Spec.Replicas.Min; got != 0 {
		t.Fatalf("expected replicas.min to default to 0, got %d", got)
	}
}

func TestHTTPScaledObjectUsesConfiguredMinReplicas(t *testing.T) {
	minScale := int64(1)
	httpScaledObject := newTestHTTPScaledObject(t, fn.Function{
		Deploy: fn.DeploySpec{
			Options: fn.Options{
				Scale: &fn.ScaleOptions{
					Min: &minScale,
				},
			},
		},
	})

	if httpScaledObject.Spec.Replicas == nil || httpScaledObject.Spec.Replicas.Min == nil {
		t.Fatal("expected replicas.min to be set")
	}

	if got := *httpScaledObject.Spec.Replicas.Min; got != int32(minScale) {
		t.Fatalf("expected replicas.min to use configured value %d, got %d", minScale, got)
	}
}

func newTestHTTPScaledObject(t *testing.T, f fn.Function) *httpv1alpha1.HTTPScaledObject {
	t.Helper()

	f.Name = "test-function"
	f.Runtime = "go"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-function",
			UID:  types.UID("test-uid"),
		},
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-function",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 8080},
			},
		},
	}

	httpScaledObject, err := NewDeployer().httpScaledObject(f, "default", deployment, service, []string{"test-function.default.svc"})
	if err != nil {
		t.Fatalf("httpScaledObject() error = %v", err)
	}

	return httpScaledObject
}
