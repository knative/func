//go:build integration
// +build integration

package k8s_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/k8s"
)

func TestUploadToVolume(t *testing.T) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	t.Cleanup(cancel)

	cliSet, testingNS, err := k8s.NewClientAndResolvedNamespace("")
	if err != nil {
		t.Fatal(err)
	}

	rnd := rand.String(5)
	testingPVCName := "testing-pvc-" + rnd

	err = k8s.CreatePersistentVolumeClaim(ctx, testingPVCName, testingNS,
		nil, nil, corev1.ReadWriteOnce,
		*resource.NewQuantity(1024, resource.DecimalSI), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pp := metav1.DeletePropagationBackground
		delOpts := metav1.DeleteOptions{
			PropagationPolicy: &pp,
		}
		_ = cliSet.CoreV1().PersistentVolumeClaims(testingNS).Delete(ctx, testingPVCName, delOpts)
	})
	t.Log("created PVC: " + testingPVCName)

	// First, test error handling by uploading empty content stream.
	err = k8s.UploadToVolume(ctx, &bytes.Buffer{}, testingPVCName, testingNS)
	if err == nil || !strings.Contains(err.Error(), "does not look like a tar") {
		t.Error("got <nil> error, or error with unexpected message")
	}

	f, err := os.Open(filepath.Join("testdata", "content.tar"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })

	err = k8s.UploadToVolume(ctx, f, testingPVCName, testingNS)
	if err != nil {
		t.Fatal(err)
	}

	testingPodName := "testing-pod-" + rnd

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testingPodName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name: "pvol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: testingPVCName,
					},
				},
			}},
			Containers: []corev1.Container{
				{
					Name:    testingPodName,
					Image:   "alpine",
					Command: []string{"cat", "/tmp/mnt/hello.txt", "/tmp/mnt/world.txt"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "pvol",
							MountPath: "/tmp/mnt/",
						},
					},
				},
			},
		},
	}

	_, err = cliSet.CoreV1().Pods(testingNS).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cliSet.CoreV1().Pods(testingNS).Delete(ctx, testingPodName, metav1.DeleteOptions{})
	})
	t.Log("created pod: " + testingPodName)

	nameSelector := fields.OneTermEqualSelector("metadata.name", testingPodName).String()
	listOpts := metav1.ListOptions{
		FieldSelector: nameSelector,
		Watch:         true,
	}
	watcher, err := cliSet.CoreV1().Pods(testingNS).Watch(ctx, listOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { watcher.Stop() })
	for event := range watcher.ResultChan() {
		if len(event.Object.(*corev1.Pod).Status.ContainerStatuses) > 0 {
			termState := event.Object.(*corev1.Pod).Status.ContainerStatuses[0].State.Terminated
			if termState != nil {
				break
			}
		}
	}
	t.Log("the testing pod has exited")

	out, err := k8s.GetPodLogs(ctx, testingNS, testingPodName, testingPodName)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Hello World!") {
		t.Error("unexpected output")
	}
}

func TestListPersistentVolumeClaimsNamesIfConnectedWrongKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/non-existent.config")
	_, err := k8s.ListPersistentVolumeClaimsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListPersistentVolumeClaimsNamesIfConnectedWrongKubernentesMaster(t *testing.T) {
	t.Setenv("KUBERNETES_MASTER", "/tmp/non-existent.config")
	_, err := k8s.ListPersistentVolumeClaimsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}
