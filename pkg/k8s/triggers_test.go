//go:build !integration

package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	"knative.dev/client/pkg/util/mock"
	fn "knative.dev/func/pkg/functions"
)

func forbiddenTriggersErr() error {
	return apierrors.NewForbidden(schema.GroupResource{Group: "eventing.knative.dev", Resource: "triggers"}, "", nil)
}

// captureStderr runs fn with os.Stderr redirected, returning what it wrote.
func captureStderr(t *testing.T, fn func()) (out string) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	outC := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		outC <- string(data)
	}()

	defer func() {
		os.Stderr = old
		w.Close()
		out = <-outC
		r.Close()
	}()

	fn()
	return
}

// Test_TriggerSync_ForbiddenWithoutSubscriptions_NoBlock: a function
// that never touches eventing (zero Deploy.Subscriptions, so syncTriggers'
// create block is skipped entirely) still runs the unconditional ListTriggers
// cleanup call, which is the only call in this scenario.
func Test_TriggerSync_ForbiddenWithoutSubscriptions_NoBlock(t *testing.T) {
	namespace := "myns"

	eventingClient := clienteventingv1.NewMockKnEventingClient(t, namespace)
	eventingClient.Recorder().ListTriggers(nil, forbiddenTriggersErr())

	f := fn.Function{Name: "myfn"}

	syncErr := syncTriggers(context.Background(), f, namespace, eventingClient, nil)
	eventingClient.Recorder().Validate()

	var err error
	stderr := captureStderr(t, func() {
		err = warnOrFailOnTriggerSync(syncErr, namespace, len(f.Deploy.Subscriptions) > 0)
	})

	if err != nil {
		t.Fatalf("expected a forbidden trigger sync not to block the deploy, got: %v", err)
	}
	if !strings.Contains(stderr, "cannot sync eventing triggers (permission denied)") {
		t.Errorf("expected an actionable warning on stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, `namespace "myns"`) {
		t.Errorf("expected the warning to name the namespace, got: %q", stderr)
	}
	if !strings.Contains(stderr, "you can safely ignore this message") {
		t.Errorf("expected the warning to reassure functions that don't use subscriptions, got: %q", stderr)
	}
}

// Test_TriggerSync_OtherErrorStillFails covers a non-forbidden list
// failure propagating all the way through the choke point to a hard
// deploy failure, with no warning printed.
func Test_TriggerSync_OtherErrorStillFails(t *testing.T) {
	namespace := "myns"

	eventingClient := clienteventingv1.NewMockKnEventingClient(t, namespace)
	eventingClient.Recorder().ListTriggers(nil, apierrors.NewInternalError(fmt.Errorf("boom")))

	f := fn.Function{Name: "myfn"}

	syncErr := syncTriggers(context.Background(), f, namespace, eventingClient, nil)
	eventingClient.Recorder().Validate()

	var err error
	stderr := captureStderr(t, func() {
		err = warnOrFailOnTriggerSync(syncErr, namespace, len(f.Deploy.Subscriptions) > 0)
	})

	if err == nil {
		t.Fatal("expected a non-forbidden trigger sync error to still fail the deploy")
	}
	if stderr != "" {
		t.Errorf("expected no warning printed on a hard error, got: %q", stderr)
	}
}

// Test_TriggerSync_ForbiddenWithSubscriptions_Fails: unlike the
// no-subscriptions case, a function that declares subscriptions is
// genuinely dependent on eventing - a forbidden trigger sync must fail
// the deploy rather than silently no-op.
func Test_TriggerSync_ForbiddenWithSubscriptions_Fails(t *testing.T) {
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

	eventingClient := clienteventingv1.NewMockKnEventingClient(t, namespace)
	eventingClient.Recorder().CreateTrigger(mock.Any(), forbiddenTriggersErr())

	syncErr := syncTriggers(context.Background(), f, namespace, eventingClient, clientset)
	eventingClient.Recorder().Validate()

	var err error
	stderr := captureStderr(t, func() {
		err = warnOrFailOnTriggerSync(syncErr, namespace, len(f.Deploy.Subscriptions) > 0)
	})

	if err == nil {
		t.Fatal("expected a forbidden trigger sync to block the deploy when subscriptions are declared")
	}
	if !strings.Contains(err.Error(), "subscriptions") || !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected the error to mention subscriptions and permission denial, got: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no warning printed when the deploy is blocked, got: %q", stderr)
	}
}
