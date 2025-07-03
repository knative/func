//go:build integration
// +build integration

package k8s_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"knative.dev/func/pkg/k8s"
)

func TestGetPodLogs(t *testing.T) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
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
