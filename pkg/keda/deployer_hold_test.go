package keda

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	fn "knative.dev/func/pkg/functions"
)

// TestDeploy_HoldsReplicaUntilReady runs the real Deploy against a fake API
// server holding a ready function whose scaler is at a given minimum, and
// checks the order of the writes (#4118).
func TestDeploy_HoldsReplicaUntilReady(t *testing.T) {
	const (
		hold    = "SET min=1"
		restore = "SET min=0"
	)
	min3 := &fn.ScaleOptions{Min: new(int64(3))}

	for _, tt := range []struct {
		name    string
		kafka   bool
		live    int64
		scale   *fn.ScaleOptions
		stale   int  // generated ScaledObject GETs that still report 0 after the hold
		failSvc bool // fail the Service write after the Deployment update
		want    []string
	}{
		{name: "http holds before the update", want: []string{hold}},
		{name: "kafka holds before the update", kafka: true, want: []string{hold}},
		{name: "http holds although min 3 is wanted", scale: min3, want: []string{hold}},
		{name: "http waits for the generated ScaledObject", stale: 2, want: []string{hold}},
		{name: "http restores after a failure", failSvc: true, want: []string{hold}},
		{name: "kafka restores after a failure", kafka: true, failSvc: true, want: []string{hold}},
		{name: "http at min 1 is untouched", live: 1},
		{name: "kafka at min 1 is untouched", kafka: true, live: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &holdServer{min: tt.live, stale: tt.stale, kafka: tt.kafka, failSvc: tt.failSvc}
			server := httptest.NewServer(s)
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err = NewDeployer().Deploy(ctx, f)
			if tt.failSvc != (err != nil) || (err != nil && !strings.Contains(err.Error(), "stop here")) {
				t.Fatalf("failSvc=%v, got error %v", tt.failSvc, err)
			}

			s.mu.Lock()
			defer s.mu.Unlock()
			// Scaler writes after the deployment update set the configured
			// minimum (or restore it), so only the earlier ones are the hold.
			var sets, generated []string
			put := -1
			for i, e := range s.events {
				switch {
				case e == "PUT deployment" && put < 0:
					put = i
				case strings.HasPrefix(e, "SET") && put < 0:
					sets = append(sets, e)
				case strings.HasPrefix(e, "GET generated") && put < 0:
					generated = append(generated, e)
				}
			}
			if put < 0 {
				t.Fatalf("deployment was never updated\nevents: %v", s.events)
			}
			if !slices.Equal(sets, tt.want) {
				t.Fatalf("scaler writes before the deployment update = %v, want %v\nevents: %v", sets, tt.want, s.events)
			}
			if tt.failSvc && s.events[len(s.events)-1] != restore {
				t.Errorf("min was not restored after the failure\nevents: %v", s.events)
			}
			if !tt.kafka && len(tt.want) > 0 {
				want := append(slices.Repeat([]string{"GET generated min=0"}, tt.stale), "GET generated min=1")
				if !slices.Equal(generated, want) {
					t.Errorf("generated ScaledObject reads before the update = %v, want %v", generated, want)
				}
			}
		})
	}
}

// holdServer is a fake API server for one ready function with a scaler at
// min. For HTTP, KEDA acts on a generated ScaledObject that mirrors the
// HTTPScaledObject only after stale reads.
type holdServer struct {
	mu      sync.Mutex
	events  []string
	min     int64
	stale   int
	kafka   bool
	failSvc bool
	patched bool
}

func (s *holdServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	status := func(code int, reason metav1.StatusReason) {
		reply(code, &metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status: "Failure", Reason: reason, Code: int32(code), Message: "stop here"})
	}
	const (
		deployment = "/apis/apps/v1/namespaces/test/deployments/f"
		service    = "/api/v1/namespaces/test/services/f"
		generated  = "/apis/keda.sh/v1alpha1/namespaces/test/scaledobjects/f"
	)
	scaler := "/apis/http.keda.sh/v1alpha1/namespaces/test/httpscaledobjects/f"
	if s.kafka {
		scaler = generated + "-kafka"
	}
	one := int32(1)
	live := map[string]any{"metadata": map[string]any{"name": "f", "namespace": "test"}}
	if s.kafka {
		live["metadata"] = map[string]any{"name": "f-kafka", "namespace": "test"}
		live["apiVersion"], live["kind"] = "keda.sh/v1alpha1", "ScaledObject"
		live["spec"] = map[string]any{"minReplicaCount": s.min}
	} else {
		live["apiVersion"], live["kind"] = "http.keda.sh/v1alpha1", "HTTPScaledObject"
		live["spec"] = map[string]any{"replicas": map[string]any{"min": s.min}}
		live["status"] = map[string]any{"conditions": []map[string]any{{"type": string(httpv1alpha1.ConditionTypeReady), "status": "True"}}}
	}
	hash := map[string]string{"pod-template-hash": "h"}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == deployment:
		reply(200, &appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "1"},
			Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}}},
			Status: appsv1.DeploymentStatus{UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}},
		})
	case r.Method == http.MethodGet && r.URL.Path == service:
		reply(200, &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "1"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}}})
	case r.Method == http.MethodGet && r.URL.Path == "/apis/apps/v1/namespaces/test/replicasets":
		reply(200, &appsv1.ReplicaSetList{Items: []appsv1.ReplicaSet{{
			ObjectMeta: metav1.ObjectMeta{Name: "rs", Labels: hash}, Spec: appsv1.ReplicaSetSpec{Replicas: &one}}}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/test/pods":
		reply(200, &corev1.PodList{Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "p", Labels: hash},
			Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}}})
	case r.Method == http.MethodGet && r.URL.Path == scaler:
		reply(200, live)
	case r.Method == http.MethodGet && r.URL.Path == generated && !s.kafka:
		got := s.min
		if s.patched && s.stale > 0 {
			s.stale--
			got = 0
		}
		s.events = append(s.events, fmt.Sprintf("GET generated min=%d", got))
		reply(200, map[string]any{"apiVersion": "keda.sh/v1alpha1", "kind": "ScaledObject",
			"metadata": map[string]any{"name": "f", "namespace": "test"},
			"spec":     map[string]any{"minReplicaCount": got}})
	case r.Method == http.MethodPut && r.URL.Path == scaler:
		var p struct {
			Spec struct {
				MinReplicaCount *int64 `json:"minReplicaCount"`
				Replicas        *struct {
					Min *int64 `json:"min"`
				} `json:"replicas"`
			} `json:"spec"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &p); err != nil {
			status(400, metav1.StatusReasonBadRequest)
			return
		}
		s.min = 0
		if p.Spec.MinReplicaCount != nil {
			s.min = *p.Spec.MinReplicaCount
		} else if p.Spec.Replicas != nil && p.Spec.Replicas.Min != nil {
			s.min = *p.Spec.Replicas.Min
		}
		s.patched = true
		s.events = append(s.events, fmt.Sprintf("SET min=%d", s.min))
		reply(200, live)
	case r.Method == http.MethodPut && r.URL.Path == service && s.failSvc:
		s.events = append(s.events, "PUT service")
		status(500, metav1.StatusReasonInternalError)
	case r.Method == http.MethodPut || r.Method == http.MethodPost:
		if r.URL.Path == deployment {
			s.events = append(s.events, "PUT deployment")
		}
		// Echo in the encoding the client sent (typed clients use protobuf).
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.WriteHeader(200)
		_, _ = w.Write(body)
	default:
		status(404, metav1.StatusReasonNotFound)
	}
}
