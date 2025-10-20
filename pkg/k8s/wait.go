package k8s

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// WaitForPodsReady waits for the specified number of pods matching the selector to be in Ready state
func WaitForPodsReady(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelSelector string, minPods int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return false, err
		}

		readyCount := 0
		for _, pod := range podList.Items {
			if pod.Status.Phase == corev1.PodRunning {
				// Check if all containers are ready
				allReady := true
				for _, status := range pod.Status.ContainerStatuses {
					if !status.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					readyCount++
				}
			}
		}

		return readyCount >= minPods, nil
	})
}
