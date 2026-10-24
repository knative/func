package keda

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func TestDeploy_ReplicasOnRedeploy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		live  int32
		scale *fn.ScaleOptions
		raw   bool
		kafka bool
		want  int32
	}{
		{name: "keda preserves live count", live: 6, want: 6},
		{name: "keda raises zero to one", live: 0, want: 1},
		{name: "keda raises minimum", live: 2, scale: &fn.ScaleOptions{Min: new(int64(5))}, want: 5},
		{name: "keda lowers maximum", live: 8, scale: &fn.ScaleOptions{Max: new(int64(3))}, want: 3},
		{name: "kafka preserves live count", live: 6, kafka: true, want: 6},
		{name: "raw uses minimum", live: 6, scale: &fn.ScaleOptions{Min: new(int64(2))}, raw: true, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			updates := make(chan *appsv1.Deployment, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/apis/apps/v1/namespaces/test/deployments/f":
					_ = json.NewEncoder(w).Encode(&appsv1.Deployment{
						TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
						ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "1"},
						Spec:       appsv1.DeploymentSpec{Replicas: &tt.live},
					})
				case r.Method == http.MethodPut && r.URL.Path == "/apis/apps/v1/namespaces/test/deployments/f":
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read deployment update: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					deployment := &appsv1.Deployment{}
					if err := runtime.DecodeInto(scheme.Codecs.UniversalDeserializer(), body, deployment); err != nil {
						t.Errorf("decode deployment update: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					updates <- deployment
					_ = json.NewEncoder(w).Encode(deployment)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/namespaces/test/services":
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(&metav1.Status{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
						Status:   "Failure", Reason: metav1.StatusReasonInternalError, Code: 500,
						Message: "stop before readiness",
					})
				default:
					if r.Method != http.MethodGet {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(&metav1.Status{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
						Status:   "Failure", Reason: metav1.StatusReasonNotFound, Code: 404,
					})
				}
			}))
			defer server.Close()

			kubeconfig := filepath.Join(t.TempDir(), "config")
			err := clientcmd.WriteToFile(clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: server.URL}},
				Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", Namespace: "test"}},
				CurrentContext: "test",
			}, kubeconfig)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("KUBECONFIG", kubeconfig)

			f := fn.Function{Name: "f", Namespace: "test", Scale: tt.scale,
				Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
			if tt.kafka {
				f.Scale = &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "kafka"}}}}
				f.Run.Kafka = &fn.KafkaConfig{Brokers: "broker:9092", Topic: "events", ConsumerGroup: "f"}
			}
			var d fn.Deployer = NewDeployer()
			if tt.raw {
				d = k8s.NewDeployer()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := d.Deploy(ctx, f); err == nil || !strings.Contains(err.Error(), "stop before readiness") {
				t.Fatalf("expected Service write failure, got %v", err)
			}
			select {
			case deployment := <-updates:
				if deployment.Spec.Replicas == nil {
					t.Fatal("deployment update has nil replicas")
				}
				if got := *deployment.Spec.Replicas; got != tt.want {
					t.Errorf("deployment update replicas = %d, want %d", got, tt.want)
				}
			default:
				t.Fatal("deployment update was not reached")
			}
		})
	}
}
