package knative

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
	"knative.dev/func/k8s"
)

// GetKServiceLogs will get logs of Knative service.
//
// It will do so by gathering logs of user-container of all affiliated pods.
// In addition, filtering on image can be done so only logs for given image are logged.
// The image must be the digest format since pods of Knative service use it.
//
// This function runs as long as the passed context is active (i.e. it is required cancel the context to stop log gathering).
func GetKServiceLogs(ctx context.Context, namespace, kServiceName, image string, since *time.Time, out io.Writer) error {
	client, namespace, err := k8s.NewClientAndResolvedNamespace(namespace)
	if err != nil {
		return fmt.Errorf("cannot create k8s client: %w", err)
	}

	pods := client.CoreV1().Pods(namespace)

	podListOpts := metav1.ListOptions{
		Watch:         true,
		LabelSelector: fmt.Sprintf("serving.knative.dev/service=%s", kServiceName),
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
			Container: "user-container",
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
			if status.Name == "user-container" {
				return status.State.Running != nil || status.State.Terminated != nil
			}
		}
		return false
	}

	getImage := func(pod corev1.Pod) string {
		for _, ctr := range pod.Spec.Containers {
			if ctr.Name == "user-container" {
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
