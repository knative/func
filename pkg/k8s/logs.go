package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// GetPodLogs returns logs from a specified Container in a Pod, if container is empty string,
// then the first container in the pod is selected.
func GetPodLogs(ctx context.Context, namespace, podName, containerName string) (string, error) {
	podLogOpts := corev1.PodLogOptions{}
	if containerName != "" {
		podLogOpts.Container = containerName
	}

	client, namespace, _ := NewClientAndResolvedNamespace(namespace)
	request := client.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)

	containerLogStream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer containerLogStream.Close()

	buffer := new(bytes.Buffer)
	_, err = io.Copy(buffer, containerLogStream)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// GetPodLogsBySelector will get logs of a pod.
//
// It will do so by gathering logs of the given container of all affiliated pods.
// In addition, filtering on image can be done so only logs for given image are logged.
//
// This function runs as long as the passed context is active (i.e. it is required cancel the context to stop log gathering).
func GetPodLogsBySelector(ctx context.Context, namespace, labelSelector, containerName, image string, since *time.Time, out io.Writer) error {
	client, namespace, err := NewClientAndResolvedNamespace(namespace)
	if err != nil {
		return fmt.Errorf("cannot create k8s client: %w", err)
	}

	pods := client.CoreV1().Pods(namespace)

	podListOpts := metav1.ListOptions{
		Watch:         true,
		LabelSelector: labelSelector,
	}

	w, err := pods.Watch(ctx, podListOpts)
	if err != nil {
		return fmt.Errorf("cannot create watch: %w", err)
	}
	defer w.Stop()

	beingProcessed := make(map[string]bool)
	var beingProcessedMu sync.Mutex

	copyLogs := func(pod corev1.Pod) error {
		defer func() {
			beingProcessedMu.Lock()
			delete(beingProcessed, pod.Name)
			beingProcessedMu.Unlock()
		}()
		podLogOpts := corev1.PodLogOptions{
			Container: containerName,
			Follow:    true,
		}
		if since != nil {
			sinceTime := metav1.NewTime(*since)
			podLogOpts.SinceTime = &sinceTime
		}
		req := client.CoreV1().Pods(namespace).GetLogs(pod.Name, &podLogOpts)

		r, e := req.Stream(ctx)
		if e != nil {
			return fmt.Errorf("cannot get stream: %w", e)
		}
		defer r.Close()
		_, e = io.Copy(out, r)
		if e != nil {
			return fmt.Errorf("error copying logs: %w", e)
		}
		return nil
	}

	mayReadLogs := func(pod corev1.Pod) bool {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == containerName {
				return status.State.Running != nil || status.State.Terminated != nil
			}
		}
		return false
	}

	getImage := func(pod corev1.Pod) string {
		for _, ctr := range pod.Spec.Containers {
			if ctr.Name == containerName {
				return ctr.Image
			}
		}
		return ""
	}

	var eg errgroup.Group

	for event := range w.ResultChan() {
		if event.Type == watch.Modified || event.Type == watch.Added {
			pod := *event.Object.(*corev1.Pod)

			beingProcessedMu.Lock()
			_, loggingAlready := beingProcessed[pod.Name]
			beingProcessedMu.Unlock()

			if !loggingAlready && (image == "" || image == getImage(pod)) && mayReadLogs(pod) {

				beingProcessedMu.Lock()
				beingProcessed[pod.Name] = true
				beingProcessedMu.Unlock()

				// Capture pod value for the goroutine to avoid closure over loop variable
				pod := pod
				eg.Go(func() error { return copyLogs(pod) })
			}
		}
	}

	err = eg.Wait()
	if err != nil {
		return fmt.Errorf("error while gathering logs: %w", err)
	}
	return nil
}
