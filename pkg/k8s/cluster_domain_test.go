package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	fn "knative.dev/func/pkg/functions"
)

// TestClusterDomain: until the domain can be resolved from a flag, the user
// config or the cluster, the helper must keep returning the Kubernetes default
// so that routing every deployer through it changes no addresses.
func TestClusterDomain(t *testing.T) {
	if got := ClusterDomain(); got != DefaultClusterDomain {
		t.Fatalf("ClusterDomain() = %q, want %q", got, DefaultClusterDomain)
	}
	if DefaultClusterDomain != "cluster.local" {
		t.Fatalf("DefaultClusterDomain = %q, want %q", DefaultClusterDomain, "cluster.local")
	}
}

// triggerCapturingClient records the Triggers syncTriggers creates and
// delegates every other call to the embedded mock.
type triggerCapturingClient struct {
	clienteventingv1.KnEventingClient
	created []*eventingv1.Trigger
}

func (c *triggerCapturingClient) CreateTrigger(_ context.Context, t *eventingv1.Trigger) error {
	c.created = append(c.created, t)
	return nil
}

// TestSyncTriggers_SubscriberUsesClusterDomain: the Trigger subscriber must
// address the function's Service through ClusterDomain(), not a hard-coded
// domain.
func TestSyncTriggers_SubscriberUsesClusterDomain(t *testing.T) {
	namespace := "myns"
	f := fn.Function{
		Name: "myfn",
		Deploy: fn.DeploySpec{
			Subscriptions: []fn.KnativeSubscription{{Source: "my-broker"}},
		},
	}

	clientset := fake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: namespace}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: namespace}},
	)

	mock := clienteventingv1.NewMockKnEventingClient(t, namespace)
	mock.Recorder().ListTriggers(&eventingv1.TriggerList{}, nil)
	client := &triggerCapturingClient{KnEventingClient: mock}

	if err := syncTriggers(context.Background(), f, namespace, client, clientset); err != nil {
		t.Fatalf("syncTriggers: %v", err)
	}
	mock.Recorder().Validate()

	if len(client.created) != 1 {
		t.Fatalf("created %d triggers, want 1", len(client.created))
	}
	uri := client.created[0].Spec.Subscriber.URI
	if uri == nil {
		t.Fatal("trigger subscriber has no URI")
	}
	want := "myfn.myns.svc." + ClusterDomain()
	if uri.Host != want {
		t.Errorf("subscriber host = %q, want %q", uri.Host, want)
	}
}
