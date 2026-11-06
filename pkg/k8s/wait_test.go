package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
)

// zeroDeployment returns a Deployment wanting zero replicas at generation 2
// with a fully reconciled all-zero status, unless mutated. The clientset is
// always nil in these tests: the zero-replica path must decide from status
// alone and never touch the API server (a nil dereference panics, so a pass
// proves no client use).
func zeroDeployment(mut func(*appsv1.Deployment)) *appsv1.Deployment {
	zero := int32(0)
	d := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &zero,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            0,
			UpdatedReplicas:     0,
			ReadyReplicas:       0,
			AvailableReplicas:   0,
			UnavailableReplicas: 0,
		},
	}
	d.Name = "fn"
	d.Namespace = "default"
	d.Generation = 2
	if mut != nil {
		mut(d)
	}
	return d
}

// TestCheckIfDeploymentIsAvailable_ZeroReplicas covers issue #4068: a live
// Deployment at 0 desired replicas (e.g. a KEDA scaler idling a function to
// zero during the post-redeploy wait) must be judged on its reconciled
// status, not fail looking for an active ReplicaSet.
func TestCheckIfDeploymentIsAvailable_ZeroReplicas(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciled zero is ready without any client use", func(t *testing.T) {
		ready, err := checkIfDeploymentIsAvailable(ctx, nil, zeroDeployment(nil))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ready {
			t.Error("expected ready=true for a reconciled zero-replica Deployment")
		}
	})

	t.Run("stale observedGeneration is not ready", func(t *testing.T) {
		// A scaler changed replicas; status still describes generation 2.
		d := zeroDeployment(func(d *appsv1.Deployment) {
			d.Generation = 3
			d.Status.ObservedGeneration = 2
		})
		ready, err := checkIfDeploymentIsAvailable(ctx, nil, d)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ready {
			t.Error("expected ready=false when status does not describe this generation")
		}
	})

	t.Run("remaining replica from an older ReplicaSet is not ready", func(t *testing.T) {
		// Scale-down not finished: one pod still counted in status.replicas
		// (it belongs to an older ReplicaSet; the other counts are already
		// zero). Must retry, not succeed.
		d := zeroDeployment(func(d *appsv1.Deployment) {
			d.Status.Replicas = 1
		})
		ready, err := checkIfDeploymentIsAvailable(ctx, nil, d)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ready {
			t.Error("expected ready=false while a replica remains")
		}
	})

	t.Run("unavailable replica is not ready", func(t *testing.T) {
		d := zeroDeployment(func(d *appsv1.Deployment) {
			d.Status.UnavailableReplicas = 1
		})
		ready, err := checkIfDeploymentIsAvailable(ctx, nil, d)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ready {
			t.Error("expected ready=false with an unavailable replica")
		}
	})
}
