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
	"k8s.io/client-go/kubernetes"
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

// PodLogsOptions are the parameters of GetPodLogsBySelector.
type PodLogsOptions struct {
	// Namespace in which the pods reside.  An empty value resolves to the
	// currently active namespace.
	Namespace string

	// LabelSelector which matches the pods whose logs are gathered.
	LabelSelector string

	// Container whose logs are gathered.
	Container string

	// Image, when non-empty, restricts gathering to pods whose container runs
	// this exact image.
	Image string

	// Since, when non-nil, is the time from which logs are returned.
	Since *time.Time

	// Follow keeps streaming the logs of matching pods, including pods which
	// appear later, until the context is cancelled.
	Follow bool
}

// GetPodLogsBySelector will get logs of a pod.
//
// It will do so by gathering logs of the given container of all affiliated pods.
// In addition, filtering on image can be done so only logs for given image are logged.
//
// This function runs as long as the passed context is active (i.e. it is
// required to cancel the context to stop log gathering).
func GetPodLogsBySelector(ctx context.Context, opts PodLogsOptions, out io.Writer) error {
	client, namespace, err := NewClientAndResolvedNamespace(opts.Namespace)
	if err != nil {
		return fmt.Errorf("cannot create k8s client: %w", err)
	}

	return followPodLogs(ctx, client, namespace, opts, out)
}

// followPodLogs streams logs of the matching pods, including those which appear
// later, until the context is cancelled.
func followPodLogs(ctx context.Context, client kubernetes.Interface, namespace string, opts PodLogsOptions, out io.Writer) error {
	podListOpts := metav1.ListOptions{
		Watch:         true,
		LabelSelector: opts.LabelSelector,
	}

	w, err := client.CoreV1().Pods(namespace).Watch(ctx, podListOpts)
	if err != nil {
		return fmt.Errorf("cannot create watch: %w", err)
	}
	defer w.Stop()

	beingProcessed := make(map[string]bool)
	var beingProcessedMu sync.Mutex

	var eg errgroup.Group

	for event := range w.ResultChan() {
		if event.Type == watch.Modified || event.Type == watch.Added {
			pod := *event.Object.(*corev1.Pod)

			beingProcessedMu.Lock()
			_, loggingAlready := beingProcessed[pod.Name]
			beingProcessedMu.Unlock()

			if !loggingAlready && (opts.Image == "" || opts.Image == containerImage(pod, opts.Container)) && mayReadLogs(pod, opts.Container) {

				beingProcessedMu.Lock()
				beingProcessed[pod.Name] = true
				beingProcessedMu.Unlock()

				// Capture pod value for the goroutine to avoid closure over loop variable
				pod := pod
				eg.Go(func() error {
					defer func() {
						beingProcessedMu.Lock()
						delete(beingProcessed, pod.Name)
						beingProcessedMu.Unlock()
					}()
					return copyPodLogs(ctx, client, namespace, pod.Name, opts, out)
				})
			}
		}
	}

	err = eg.Wait()
	if err != nil {
		return fmt.Errorf("error while gathering logs: %w", err)
	}
	return nil
}

// copyPodLogs writes the logs of a single pod's container to out.
func copyPodLogs(ctx context.Context, client kubernetes.Interface, namespace, podName string, opts PodLogsOptions, out io.Writer) error {
	podLogOpts := corev1.PodLogOptions{
		Container: opts.Container,
		Follow:    opts.Follow,
	}
	if opts.Since != nil {
		sinceTime := metav1.NewTime(*opts.Since)
		podLogOpts.SinceTime = &sinceTime
	}

	r, err := client.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts).Stream(ctx)
	if err != nil {
		return fmt.Errorf("cannot get stream: %w", err)
	}
	defer r.Close()
	if _, err = io.Copy(out, r); err != nil {
		return fmt.Errorf("error copying logs: %w", err)
	}
	return nil
}

// mayReadLogs returns whether the given container of the pod has produced logs
// which can be read.
func mayReadLogs(pod corev1.Pod, containerName string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
			return status.State.Running != nil || status.State.Terminated != nil
		}
	}
	return false
}

// containerImage returns the image of the given container of the pod.
func containerImage(pod corev1.Pod, containerName string) string {
	for _, ctr := range pod.Spec.Containers {
		if ctr.Name == containerName {
			return ctr.Image
		}
	}
	return ""
}

type SynchronizedBuffer struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (b *SynchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *SynchronizedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *SynchronizedBuffer) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Read(p)
}

func (b *SynchronizedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b.Reset()
}
