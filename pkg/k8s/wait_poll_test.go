package k8s_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"knative.dev/func/pkg/k8s"
)

func TestWaitForDeploymentAvailable_ScaleToZero(t *testing.T) {
	zero := waitDeployment(0)
	waking := waitDeployment(1)
	waking.Generation = 6
	waking.Status.ObservedGeneration = 6
	waking.Status.ReadyReplicas = 0
	waking.Status.AvailableReplicas = 0
	waking.Status.Conditions[0].Status = corev1.ConditionFalse
	stale := zero.DeepCopy()
	stale.Status.ObservedGeneration--
	remaining := zero.DeepCopy()
	remaining.Status.Replicas = 1
	unavailable := zero.DeepCopy()
	unavailable.Status.UnavailableReplicas = 1

	for _, tt := range []struct {
		name       string
		states     []*appsv1.Deployment
		bySelector bool
	}{
		{name: "reconciled zero without an active ReplicaSet", states: []*appsv1.Deployment{zero}},
		{name: "selector wait", states: []*appsv1.Deployment{zero}, bySelector: true},
		{name: "scaler zeroes a waking deployment", states: []*appsv1.Deployment{waking, stale, zero}},
		{name: "remaining old replica", states: []*appsv1.Deployment{remaining, zero}},
		{name: "unavailable replica", states: []*appsv1.Deployment{unavailable, zero}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &waitAPI{deployments: tt.states}
			client := api.client(t)
			var err error
			if tt.bySelector {
				err = k8s.WaitForDeploymentAvailableBySelector(t.Context(), client, "ns", "app=fn", 10*time.Second)
			} else {
				err = k8s.WaitForDeploymentAvailable(t.Context(), client, "ns", "fn", 10*time.Second)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := api.deploymentReads.Load(); got != int32(len(tt.states)) {
				t.Errorf("deployment reads = %d, want %d", got, len(tt.states))
			}
			if api.replicaSetReads.Load() != 0 || api.podReads.Load() != 0 {
				t.Fatal("zero-replica readiness queried ReplicaSets or pods")
			}
		})
	}
}

func TestWaitForDeploymentAvailable_ReplicaSetChangesBetweenReads(t *testing.T) {
	for _, tt := range []struct {
		name   string
		states []*appsv1.Deployment
	}{
		{name: "scaler zeroes the ReplicaSet", states: []*appsv1.Deployment{waitDeployment(1), waitDeployment(0)}},
		{name: "active ReplicaSet appears on next poll", states: []*appsv1.Deployment{waitDeployment(1), waitDeployment(1)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &waitAPI{deployments: tt.states, activeAfter: 2}
			err := k8s.WaitForDeploymentAvailable(t.Context(), api.client(t), "ns", "fn", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if api.deploymentReads.Load() != 2 {
				t.Fatal("inconsistent ReplicaSet read was not retried")
			}
		})
	}
}

func TestWaitForDeploymentAvailable_Errors(t *testing.T) {
	stale := waitDeployment(0)
	stale.Status.ObservedGeneration--
	for _, tt := range []struct {
		name          string
		deployment    *appsv1.Deployment
		replicaStatus int
	}{
		{name: "stale zero times out", deployment: stale},
		{name: "missing active ReplicaSet times out", deployment: waitDeployment(1)},
		{name: "ReplicaSet API error", deployment: waitDeployment(1), replicaStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &waitAPI{deployments: []*appsv1.Deployment{tt.deployment}, replicaStatus: tt.replicaStatus}
			timeout := 100 * time.Millisecond
			if tt.replicaStatus != 0 {
				timeout = 5 * time.Second
			}
			err := k8s.WaitForDeploymentAvailable(t.Context(), api.client(t), "ns", "fn", timeout)
			if tt.replicaStatus != 0 {
				if !apierrors.IsForbidden(err) {
					t.Fatalf("error = %v, want forbidden", err)
				}
			} else if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v, want deadline exceeded", err)
			}
		})
	}
}

func TestWaitForDeploymentAvailable_Cancelled(t *testing.T) {
	stale := waitDeployment(0)
	stale.Status.ObservedGeneration--
	api := &waitAPI{deployments: []*appsv1.Deployment{stale}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	if err := k8s.WaitForDeploymentAvailable(ctx, api.client(t), "ns", "fn", 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
}

func TestWaitForDeploymentAvailable_NonzeroReady(t *testing.T) {
	api := &waitAPI{deployments: []*appsv1.Deployment{waitDeployment(1)}, activeAfter: 1}
	if err := k8s.WaitForDeploymentAvailable(t.Context(), api.client(t), "ns", "fn", time.Second); err != nil {
		t.Fatal(err)
	}
	if api.podReads.Load() != 1 {
		t.Fatal("nonzero readiness did not check pods")
	}
}

func waitDeployment(replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "fn", Namespace: "ns", Generation: 7},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "fn"}}},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 7, Replicas: replicas, UpdatedReplicas: replicas, ReadyReplicas: replicas, AvailableReplicas: replicas,
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
}

// Deployment and ReplicaSet reads can observe different controller snapshots.
type waitAPI struct {
	deployments     []*appsv1.Deployment
	activeAfter     int32
	replicaStatus   int
	deploymentReads atomic.Int32
	replicaSetReads atomic.Int32
	podReads        atomic.Int32
}

func (a *waitAPI) client(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	server := httptest.NewServer(a)
	t.Cleanup(server.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (a *waitAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response any
	switch r.URL.Path {
	case "/apis/apps/v1/namespaces/ns/deployments/fn", "/apis/apps/v1/namespaces/ns/deployments":
		index := min(int(a.deploymentReads.Add(1))-1, len(a.deployments)-1)
		d := *a.deployments[index]
		if strings.HasSuffix(r.URL.Path, "/fn") {
			response = d
		} else {
			response = appsv1.DeploymentList{Items: []appsv1.Deployment{d}}
		}
	case "/apis/apps/v1/namespaces/ns/replicasets":
		a.replicaSetReads.Add(1)
		if a.replicaStatus != 0 {
			w.WriteHeader(a.replicaStatus)
			response = metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: int32(a.replicaStatus), Message: "forbidden"}
			break
		}
		replicas := int32(0)
		if a.activeAfter != 0 && a.deploymentReads.Load() >= a.activeAfter {
			replicas = 1
		}
		response = appsv1.ReplicaSetList{Items: []appsv1.ReplicaSet{{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"pod-template-hash": "abc"}},
			Spec:       appsv1.ReplicaSetSpec{Replicas: &replicas},
		}}}
	case "/api/v1/namespaces/ns/pods":
		a.podReads.Add(1)
		response = corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"pod-template-hash": "abc"}},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}}}
	default:
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}
