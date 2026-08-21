//go:build integration

package k8s_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"knative.dev/func/pkg/k8s"
)

func TestInt_GetPodLogs(t *testing.T) {
	var err error
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute*5)
	t.Cleanup(cancel)
	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	testingNS := "pod-logs-test-ns-" + rand.String(5)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testingNS,
		},
		Spec: corev1.NamespaceSpec{},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.CoreV1().Namespaces().Delete(ctx, testingNS, metav1.DeleteOptions{})
	})
	t.Log("created namespace: ", testingNS)

	testingPodName := "testing-pod"

	testMsg := "Hello World!"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testingPodName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    testingPodName,
					Image:   "alpine",
					Command: []string{"echo", "-n", testMsg},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err = cliSet.CoreV1().Pods(testingNS).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("created pod: " + testingPodName)

out:
	for i := 0; i < 600; i++ {
		pod, err = cliSet.CoreV1().Pods(testingNS).Get(ctx, testingPodName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, stat := range pod.Status.ContainerStatuses {
			if stat.State.Terminated != nil {
				break out
			}
		}
		time.Sleep(time.Millisecond * 500)
	}

	out, err := k8s.GetPodLogs(ctx, testingNS, testingPodName, testingPodName)
	if err != nil {
		t.Fatal(err)
	}
	if out != testMsg {
		t.Errorf("unexpected logs: expected %q, but got %q", testMsg, out)
	}
}

// TestInt_GetPodLogsBySelectorSnapshot ensures a snapshot of the logs of the
// pods matching a selector is written and that the call returns, i.e. that it
// does not wait for further logs as following does.
func TestInt_GetPodLogsBySelectorSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute*5)
	t.Cleanup(cancel)
	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	testingNS := "pod-logs-snapshot-test-ns-" + rand.String(5)
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testingNS},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.CoreV1().Namespaces().Delete(context.Background(), testingNS, metav1.DeleteOptions{})
	})
	t.Log("created namespace: ", testingNS)

	const (
		testingPodName = "testing-pod"
		containerName  = "testing-container"
		testMsg        = "Hello World!"
	)
	opts := k8s.PodLogsOptions{
		Namespace:     testingNS,
		LabelSelector: "function.knative.dev/name=testing",
		Container:     containerName,
	}

	// A snapshot of a selector which matches no pods is not an error, but is
	// reported such that it is distinguishable from a pod logging nothing.
	err = k8s.GetPodLogsBySelector(ctx, opts, io.Discard)
	if !errors.Is(err, k8s.ErrNoMatchingPods) {
		t.Errorf("expected ErrNoMatchingPods for a selector matching no pods, got %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testingPodName,
			Labels: map[string]string{"function.knative.dev/name": "testing"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    containerName,
					Image:   "alpine",
					Command: []string{"echo", "-n", testMsg},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if _, err = cliSet.CoreV1().Pods(testingNS).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Log("created pod: " + testingPodName)

out:
	for i := 0; i < 600; i++ {
		pod, err = cliSet.CoreV1().Pods(testingNS).Get(ctx, testingPodName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, stat := range pod.Status.ContainerStatuses {
			if stat.State.Terminated != nil {
				break out
			}
		}
		time.Sleep(time.Millisecond * 500)
	}

	// The snapshot is expected to complete on its own: a context which is
	// cancelled would indicate it waited for logs which never came.
	buff := &k8s.SynchronizedBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- k8s.GetPodLogsBySelector(ctx, opts, buff)
	}()

	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatal("snapshot did not complete")
	}

	// A single pod's logs are written verbatim, with a terminating newline.
	if expected := testMsg + "\n"; buff.String() != expected {
		t.Errorf("unexpected logs: expected %q, but got %q", expected, buff.String())
	}
}
