package keda

import (
	"context"
	"encoding/json"
	"errors"
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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
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
	mu                sync.Mutex
	events            []string
	min               int64
	stale             int
	zeroPolls         int  // post-update Deployment GETs returning reconciled zero before scaling back to 1
	stayZero          bool // post-update Deployment GETs always return reconciled zero
	staleGenPolls     int  // post-update Deployment GETs returning Generation=2, ObservedGeneration=1 with old pod
	stayStaleGen      bool // post-update Deployment GETs always return stale generation
	updatedDeploy     bool
	kafka             bool
	failSvc           bool
	failSecondSvcPut  bool // fail the 2nd Service PUT (RecordExposure)
	svcPuts           int
	hsoNotReady       bool // report HTTPScaledObject Ready=False after HSO update
	exactClusterLocal bool // serve full HTTPScaledObject spec matching deployClusterLocal so DeepEqual is true
	svcAnnotations    map[string]string
	patched           bool
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
		proxySvc   = "/api/v1/namespaces/keda/services/keda-add-ons-http-interceptor-proxy"
	)
	scaler := "/apis/http.keda.sh/v1alpha1/namespaces/test/httpscaledobjects/f"
	if s.kafka {
		scaler = generated + "-kafka"
	}
	one := int32(1)
	zero := int32(0)
	readyStatus := "True"
	if s.hsoNotReady {
		readyStatus = "False"
	}
	var live any
	if s.kafka {
		live = map[string]any{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata":   map[string]any{"name": "f-kafka", "namespace": "test"},
			"spec":       map[string]any{"minReplicaCount": s.min},
		}
	} else if s.exactClusterLocal {
		live = &httpv1alpha1.HTTPScaledObject{
			TypeMeta:   metav1.TypeMeta{APIVersion: "http.keda.sh/v1alpha1", Kind: "HTTPScaledObject"},
			ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test"},
			Spec: httpv1alpha1.HTTPScaledObjectSpec{
				Hosts: bridgeHosts(deployer.NewExposureRef("f", "test", "keda")),
				ScaleTargetRef: httpv1alpha1.ScaleTargetRef{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "f",
					Service:    "f",
					Port:       80,
				},
				Replicas: &httpv1alpha1.ReplicaStruct{
					Min: new(int32(s.min)),
					Max: new(defaultMaxReplicas),
				},
				CooldownPeriod: new(int32(300)),
				ScalingMetric: &httpv1alpha1.ScalingMetricSpec{
					Rate: &httpv1alpha1.RateMetricSpec{
						TargetValue: 100,
						Window:      metav1.Duration{Duration: time.Minute},
						Granularity: metav1.Duration{Duration: time.Second},
					},
				},
			},
			Status: httpv1alpha1.HTTPScaledObjectStatus{
				Conditions: []metav1.Condition{{
					Type:   httpv1alpha1.ConditionTypeReady,
					Status: metav1.ConditionStatus(readyStatus),
				}},
			},
		}
	} else {
		live = map[string]any{
			"apiVersion": "http.keda.sh/v1alpha1",
			"kind":       "HTTPScaledObject",
			"metadata":   map[string]any{"name": "f", "namespace": "test"},
			"spec":       map[string]any{"replicas": map[string]any{"min": s.min}},
			"status":     map[string]any{"conditions": []map[string]any{{"type": string(httpv1alpha1.ConditionTypeReady), "status": readyStatus}}},
		}
	}
	hash := map[string]string{"pod-template-hash": "h"}
	if s.updatedDeploy && !s.stayStaleGen && s.staleGenPolls == 0 {
		hash = map[string]string{"pod-template-hash": "h-new"}
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == proxySvc:
		reply(200, &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: interceptorServiceName, Namespace: "keda"}})
	case r.Method == http.MethodGet && r.URL.Path == deployment:
		if s.updatedDeploy && (s.stayZero || s.zeroPolls > 0) {
			if s.zeroPolls > 0 {
				s.zeroPolls--
			}
			s.events = append(s.events, "GET deployment replicas=0")
			reply(200, &appsv1.Deployment{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "2", Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &zero, Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}}},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2},
			})
			return
		}
		if s.updatedDeploy && (s.stayStaleGen || s.staleGenPolls > 0) {
			if s.staleGenPolls > 0 {
				s.staleGenPolls--
			}
			s.events = append(s.events, "GET deployment gen=2 observed=1")
			reply(200, &appsv1.Deployment{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "2", Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}}},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}},
			})
			return
		}
		gen := int64(1)
		if s.updatedDeploy {
			gen = 2
			s.events = append(s.events, "GET deployment replicas=1")
		}
		reply(200, &appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "1", Generation: gen},
			Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}}},
			Status: appsv1.DeploymentStatus{ObservedGeneration: gen, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}},
		})
	case r.Method == http.MethodGet && r.URL.Path == service:
		reply(200, &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "test", ResourceVersion: "1", Annotations: s.svcAnnotations},
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
	case r.Method == http.MethodPut && r.URL.Path == service:
		s.svcPuts++
		s.events = append(s.events, "PUT service")
		if s.failSvc || (s.failSecondSvcPut && s.svcPuts >= 2) {
			status(500, metav1.StatusReasonInternalError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.WriteHeader(200)
		_, _ = w.Write(body)
	case r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodPatch:
		if r.URL.Path == deployment && r.Method == http.MethodPut {
			s.updatedDeploy = true
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

type fakeHoldExposer struct {
	host        string
	exposeErr   error
	unexposeErr error
}

func (e fakeHoldExposer) Expose(context.Context, dynamic.Interface, deployer.Exposure) (string, error) {
	return e.host, e.exposeErr
}

func (e fakeHoldExposer) Unexpose(context.Context, dynamic.Interface, deployer.ExposureRef) error {
	return e.unexposeErr
}

func setupHoldKubeconfig(t *testing.T, serverURL string) {
	t.Helper()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	err := clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: serverURL}},
		Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", Namespace: "test"}},
		CurrentContext: "test",
	}, kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
}

// TestDeploy_WaitRefusesSettledZero verifies that KEDA's readiness wait keeps
// polling if a stale KEDA loop scales the Deployment to 0 during the hold,
// waiting until KEDA brings a pod back, while raw still accepts settled zero.
func TestDeploy_WaitRefusesSettledZero(t *testing.T) {
	t.Run("http waits through settled zero until ready pod runs", func(t *testing.T) {
		s := &holdServer{min: 0, zeroPolls: 2}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := NewDeployer().Deploy(ctx, f); err != nil {
			t.Fatalf("Deploy() error = %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		var zeroSeen, readySeen int
		for _, e := range s.events {
			switch e {
			case "GET deployment replicas=0":
				zeroSeen++
			case "GET deployment replicas=1":
				readySeen++
			}
		}
		if zeroSeen != 2 || readySeen == 0 {
			t.Fatalf("expected 2 zero polls followed by ready pod read, got zeroSeen=%d readySeen=%d\nevents: %v", zeroSeen, readySeen, s.events)
		}
	})

	t.Run("kafka waits through settled zero until ready pod runs", func(t *testing.T) {
		s := &holdServer{min: 0, kafka: true, zeroPolls: 2}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{
			Name:      "f",
			Namespace: "test",
			Scale:     &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{{Type: "kafka"}}}},
			Run:       fn.RunSpec{Kafka: &fn.KafkaConfig{Brokers: "broker:9092", Topic: "events", ConsumerGroup: "f"}},
			Deploy:    fn.DeploySpec{Image: "registry.example.com/f:latest"},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := NewDeployer().Deploy(ctx, f); err != nil {
			t.Fatalf("Deploy() error = %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		var zeroSeen, readySeen int
		for _, e := range s.events {
			switch e {
			case "GET deployment replicas=0":
				zeroSeen++
			case "GET deployment replicas=1":
				readySeen++
			}
		}
		if zeroSeen != 2 || readySeen == 0 {
			t.Fatalf("expected 2 zero polls followed by ready pod read, got zeroSeen=%d readySeen=%d\nevents: %v", zeroSeen, readySeen, s.events)
		}
	})

	t.Run("keda times out and restores min when deployment stays at settled zero", func(t *testing.T) {
		s := &holdServer{min: 0, stayZero: true}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_, err := NewDeployer().Deploy(ctx, f)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded when deployment stays at zero, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.events) == 0 || s.events[len(s.events)-1] != "SET min=0" {
			t.Errorf("expected min=0 restore after timeout, events: %v", s.events)
		}
	})

	t.Run("raw deployer still accepts settled zero", func(t *testing.T) {
		s := &holdServer{stayZero: true}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := k8s.NewDeployer().Deploy(ctx, f); err != nil {
			t.Fatalf("raw Deploy() should accept settled zero, got %v", err)
		}
	})
}

// TestDeploy_RestoreDisarmedAfterScalerReconcile verifies that once
// ensureHTTPScaledObject has reconciled the configured minimum, a later
// failure does not roll a configured scale.min=1 back to 0.
func TestDeploy_RestoreDisarmedAfterScalerReconcile(t *testing.T) {
	min1 := &fn.ScaleOptions{Min: new(int64(1))}
	min1Max5 := &fn.ScaleOptions{Min: new(int64(1)), Max: new(int64(5))}

	t.Run("clearExposure failure after HSO reconcile keeps configured min=1", func(t *testing.T) {
		s := &holdServer{
			min:            0,
			svcAnnotations: map[string]string{k8s.RouteNamespaceAnnotation: "keda"},
		}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test", Scale: min1,
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d := NewDeployer(WithExposer(fakeHoldExposer{unexposeErr: errors.New("unexpose boom")}))
		_, err := d.Deploy(ctx, f)
		if err == nil || !strings.Contains(err.Error(), "unexpose boom") {
			t.Fatalf("expected unexpose error, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if slices.Contains(s.events, "SET min=0") {
			t.Errorf("configured min=1 was rolled back to 0 after clearExposure failure\nevents: %v", s.events)
		}
	})

	t.Run("RecordExposure failure after HSO reconcile keeps configured min=1", func(t *testing.T) {
		s := &holdServer{min: 0, failSecondSvcPut: true}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test", Scale: min1, Expose: "route",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d := NewDeployer(WithExposer(fakeHoldExposer{host: "f.example.com"}))
		_, err := d.Deploy(ctx, f)
		if err == nil || !strings.Contains(err.Error(), "recording the exposure failed") {
			t.Fatalf("expected RecordExposure error, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if slices.Contains(s.events, "SET min=0") {
			t.Errorf("configured min=1 was rolled back to 0 after RecordExposure failure\nevents: %v", s.events)
		}
	})

	t.Run("HSO readiness timeout after Update keeps configured min=1", func(t *testing.T) {
		s := &holdServer{min: 0, hsoNotReady: true}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test", Scale: min1Max5,
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_, err := NewDeployer().Deploy(ctx, f)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded waiting for HSO, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if slices.Contains(s.events, "SET min=0") {
			t.Errorf("configured min=1 was rolled back to 0 after HSO update succeeded\nevents: %v", s.events)
		}
	})

	t.Run("clearExposure failure after HSO DeepEqual no-op keeps configured min=1", func(t *testing.T) {
		s := &holdServer{
			min:               0,
			exactClusterLocal: true,
			svcAnnotations:    map[string]string{k8s.RouteNamespaceAnnotation: "keda"},
		}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test", Scale: min1,
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d := NewDeployer(WithExposer(fakeHoldExposer{unexposeErr: errors.New("unexpose boom")}))
		_, err := d.Deploy(ctx, f)
		if err == nil || !strings.Contains(err.Error(), "unexpose boom") {
			t.Fatalf("expected unexpose error, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		var sets []string
		for _, e := range s.events {
			if strings.HasPrefix(e, "SET ") {
				sets = append(sets, e)
			}
		}
		// Only the initial hold write should have occurred: ensureHTTPScaledObject
		// took the DeepEqual branch and disarmed the restore before clearExposure failed.
		if !slices.Equal(sets, []string{"SET min=1"}) {
			t.Errorf("scaler writes = %v, want only [SET min=1]\nevents: %v", sets, s.events)
		}
	})

	t.Run("Expose failure before HSO reconcile still restores min=0", func(t *testing.T) {
		s := &holdServer{min: 0}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test", Scale: min1, Expose: "route",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d := NewDeployer(WithExposer(fakeHoldExposer{exposeErr: errors.New("expose boom")}))
		_, err := d.Deploy(ctx, f)
		if err == nil || !strings.Contains(err.Error(), "expose boom") {
			t.Fatalf("expected expose error, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.events) == 0 || s.events[len(s.events)-1] != "SET min=0" {
			t.Errorf("expected min=0 restore when Expose fails before HSO reconcile\nevents: %v", s.events)
		}
	})
}

// TestDeploy_WaitIgnoresStalePreviousRevision verifies that after a Deployment
// update increments Generation, stale ready counts and old ready pods from the
// previous generation cannot satisfy the readiness wait (#4118).
func TestDeploy_WaitIgnoresStalePreviousRevision(t *testing.T) {
	t.Run("waits through stale generation until new revision is observed and ready", func(t *testing.T) {
		s := &holdServer{min: 0, staleGenPolls: 2}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:v2"}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := NewDeployer().Deploy(ctx, f); err != nil {
			t.Fatalf("Deploy() error = %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		var staleSeen, readySeen int
		for _, e := range s.events {
			switch e {
			case "GET deployment gen=2 observed=1":
				staleSeen++
			case "GET deployment replicas=1":
				readySeen++
			}
		}
		if staleSeen != 2 || readySeen == 0 {
			t.Fatalf("expected 2 stale-generation polls followed by new revision ready read, got staleSeen=%d readySeen=%d\nevents: %v", staleSeen, readySeen, s.events)
		}
	})

	t.Run("times out and restores min when observedGeneration stays behind", func(t *testing.T) {
		s := &holdServer{min: 0, stayStaleGen: true}
		server := httptest.NewServer(s)
		defer server.Close()
		setupHoldKubeconfig(t, server.URL)

		f := fn.Function{Name: "f", Namespace: "test",
			Deploy: fn.DeploySpec{Image: "registry.example.com/f:broken"}}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_, err := NewDeployer().Deploy(ctx, f)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded when observedGeneration stays stale, got %v", err)
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.events) == 0 || s.events[len(s.events)-1] != "SET min=0" {
			t.Errorf("expected min=0 restore after stale-generation timeout, events: %v", s.events)
		}
	})
}
