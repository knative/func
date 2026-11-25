package keda

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// TestDeploy_RetriesDeploymentUpdateOnConflict simulates a concurrent update
// (e.g. an autoscaler scaling the Deployment) between the deployer's GET and
// PUT: the first update is rejected with a 409 and the live object moves on.
// The deploy must re-read the live Deployment and retry, while non-conflict
// errors fail immediately without retrying.
func TestDeploy_RetriesDeploymentUpdateOnConflict(t *testing.T) {
	for _, tt := range []struct {
		name              string
		raw               bool
		scale             *fn.ScaleOptions
		failWithInternal  bool
		replaceDeployment bool
		wantUpdates       int
		wantFirstRep      int32
		wantSecondRep     int32
		wantErr           string
	}{
		{
			name:          "keda retries with live replicas on conflict",
			wantUpdates:   2,
			wantFirstRep:  3,
			wantSecondRep: 7,
			wantErr:       "stop before readiness",
		},
		{
			name:          "raw retries with configured minimum on conflict",
			raw:           true,
			scale:         &fn.ScaleOptions{Min: new(int64(2))},
			wantUpdates:   2,
			wantFirstRep:  2,
			wantSecondRep: 2,
			wantErr:       "stop before readiness",
		},
		{
			name:             "non-conflict error does not retry",
			failWithInternal: true,
			wantUpdates:      1,
			wantFirstRep:     3,
			wantErr:          "boom",
		},
		{
			name:              "replacement deployment is not updated",
			replaceDeployment: true,
			wantUpdates:       1,
			wantFirstRep:      3,
			wantErr:           `deployment "f" was replaced during update`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mu       sync.Mutex
				replicas = int32(3)
				version  = "1"
				uid      = types.UID("original")
				updates  []*appsv1.Deployment
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/apis/apps/v1/namespaces/test/deployments/f":
					rep := replicas
					_ = json.NewEncoder(w).Encode(&appsv1.Deployment{
						TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
						ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: version, UID: uid},
						Spec:       appsv1.DeploymentSpec{Replicas: &rep},
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
					updates = append(updates, deployment)
					if tt.failWithInternal {
						w.WriteHeader(http.StatusInternalServerError)
						_ = json.NewEncoder(w).Encode(&metav1.Status{
							TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
							Status:   "Failure", Reason: metav1.StatusReasonInternalError, Code: 500,
							Message: "boom",
						})
						return
					}
					if len(updates) == 1 {
						// The autoscaler wins the race.
						replicas, version = 7, "2"
						if tt.replaceDeployment {
							uid = "replacement"
						}
						w.WriteHeader(http.StatusConflict)
						_ = json.NewEncoder(w).Encode(apierrors.NewConflict(
							schema.GroupResource{Group: "apps", Resource: "deployments"}, "f",
							errors.New("the object has been modified")).Status())
						return
					}
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
			var d fn.Deployer = NewDeployer()
			if tt.raw {
				d = k8s.NewDeployer()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := d.Deploy(ctx, f); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(updates) != tt.wantUpdates {
				t.Fatalf("got %d deployment updates, want %d", len(updates), tt.wantUpdates)
			}
			if updates[0].ResourceVersion != "1" || *updates[0].Spec.Replicas != tt.wantFirstRep {
				t.Errorf("first update = rv %q replicas %d, want rv 1 replicas %d",
					updates[0].ResourceVersion, *updates[0].Spec.Replicas, tt.wantFirstRep)
			}
			if tt.wantUpdates > 1 && (updates[1].ResourceVersion != "2" || *updates[1].Spec.Replicas != tt.wantSecondRep) {
				t.Errorf("second update = rv %q replicas %d, want rv 2 replicas %d",
					updates[1].ResourceVersion, *updates[1].Spec.Replicas, tt.wantSecondRep)
			}
		})
	}
}
