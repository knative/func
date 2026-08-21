package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
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

// ErrNoMatchingPods is returned when no pod whose logs can be read matches the
// given options.  This is an expected state rather than a failure: a Knative
// service which has scaled to zero has no pods.
var ErrNoMatchingPods = errors.New("no pods with readable logs matched")

// PartialLogsError is returned when the logs of some, but not all, matching
// pods could be gathered.  The logs which were gathered have been written.
type PartialLogsError struct {
	Err error
}

func (e *PartialLogsError) Error() string {
	return fmt.Sprintf("logs of some pods could not be gathered: %v", e.Err)
}

func (e *PartialLogsError) Unwrap() error { return e.Err }

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

	// TailLines, when non-nil, limits the output to the last N lines per pod.
	TailLines *int64

	// Follow streams logs of matching pods, including pods which appear later,
	// until the context is cancelled.  When false, the logs currently available
	// from the currently matching pods are written and the call returns.
	Follow bool
}

// GetPodLogsBySelector will get logs of a pod.
//
// It will do so by gathering logs of the given container of all affiliated pods.
// In addition, filtering on image can be done so only logs for given image are logged.
//
// When Follow is set, this function runs as long as the passed context is active
// (i.e. it is required to cancel the context to stop log gathering).  Otherwise a
// snapshot of the currently available logs is written and the function returns.
func GetPodLogsBySelector(ctx context.Context, opts PodLogsOptions, out io.Writer) error {
	client, namespace, err := NewClientAndResolvedNamespace(opts.Namespace)
	if err != nil {
		return fmt.Errorf("cannot create k8s client: %w", err)
	}

	if opts.Follow {
		return followPodLogs(ctx, client, namespace, opts, out)
	}
	return snapshotPodLogs(ctx, client, namespace, opts, out)
}

// snapshotPodLogs writes the logs currently available from the pods which
// currently match the selector, then returns.
//
// Returns ErrNoMatchingPods if no pod with readable logs matches, and a
// PartialLogsError if the logs of at least one, but not all, matching pods
// could be gathered.
func snapshotPodLogs(ctx context.Context, client kubernetes.Interface, namespace string, opts PodLogsOptions, out io.Writer) error {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: opts.LabelSelector,
	})
	if err != nil {
		return fmt.Errorf("cannot list pods: %w", err)
	}

	pods := make([]corev1.Pod, 0, len(list.Items))
	for _, pod := range list.Items {
		if (opts.Image == "" || opts.Image == containerImage(pod, opts.Container)) && mayReadLogs(pod, opts.Container) {
			pods = append(pods, pod)
		}
	}
	if len(pods) == 0 {
		return ErrNoMatchingPods
	}
	// Sorted such that output is deterministic when multiple pods match.
	sort.Slice(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })

	// The logs of a single pod are written verbatim.  When several pods match,
	// their lines are attributed, as they are otherwise indistinguishable.
	prefix := ""

	var errs []error
	gathered := 0
	for _, pod := range pods {
		if len(pods) > 1 {
			prefix = fmt.Sprintf("[pod/%s] ", pod.Name)
		}
		// Lines of a pod are always terminated, such that the logs of the next
		// pod do not continue the last line of the previous.
		w := &linePrefixer{out: out, prefix: []byte(prefix)}
		if err = copyPodLogs(ctx, client, namespace, pod.Name, opts, w); err != nil {
			// A pod can, for example, be evicted between listing and reading.
			// Gather what remains rather than discarding it.
			errs = append(errs, err)
			continue
		}
		if err = w.terminate(); err != nil {
			errs = append(errs, err)
			continue
		}
		gathered++
	}

	if len(errs) == 0 {
		return nil
	}
	if gathered == 0 {
		return errors.Join(errs...)
	}
	return &PartialLogsError{Err: errors.Join(errs...)}
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
		TailLines: opts.TailLines,
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

// linePrefixer writes each line it receives prefixed, and tracks whether the
// last line written was terminated.  A pod's logs arrive in arbitrarily sized
// chunks, and its last line need not be terminated at all.
type linePrefixer struct {
	out    io.Writer
	prefix []byte
	midway bool // a line has been started but not yet terminated
}

func (l *linePrefixer) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		if !l.midway {
			if len(l.prefix) > 0 {
				if _, err := l.out.Write(l.prefix); err != nil {
					return written, err
				}
			}
			l.midway = true
		}
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			n, err := l.out.Write(p)
			return written + n, err
		}
		n, err := l.out.Write(p[:i+1])
		written += n
		if err != nil {
			return written, err
		}
		l.midway = false
		p = p[i+1:]
	}
	return written, nil
}

// terminate ends the current line if it was left unterminated.
func (l *linePrefixer) terminate() error {
	if !l.midway {
		return nil
	}
	l.midway = false
	if _, err := l.out.Write([]byte{'\n'}); err != nil {
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
