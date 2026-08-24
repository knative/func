package k8s

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// fake.Clientset always serves this as the content of a pod's logs.
const fakeLogs = "fake logs"

// runningPod returns a pod which matches the selector used by the tests and
// whose container has readable logs.
func runningPod(name, image string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "user-container", Image: image}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "user-container",
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}

// pendingPod returns a pod whose container has not yet started, and whose logs
// can therefore not be read.
func pendingPod(name string) *corev1.Pod {
	pod := runningPod(name, "example.com/img:latest")
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "user-container",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}},
	}}
	return pod
}

func snapshotOpts() PodLogsOptions {
	return PodLogsOptions{
		Namespace:     "default",
		LabelSelector: "app=test",
		Container:     "user-container",
	}
}

// TestSnapshotPodLogs_NoMatchingPods ensures that the absence of pods whose
// logs can be read is reported rather than presented as empty logs.  A Knative
// service which has scaled to zero has no pods at all, which is the common
// case, and a pod which has not yet started has no logs to read.
func TestSnapshotPodLogs_NoMatchingPods(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
		opts    func(PodLogsOptions) PodLogsOptions
	}{
		{
			name:    "no pods at all",
			objects: nil,
		},
		{
			name:    "container not yet running",
			objects: []runtime.Object{pendingPod("pod-a")},
		},
		{
			name:    "no pod runs the requested image",
			objects: []runtime.Object{runningPod("pod-a", "example.com/img:v1")},
			opts: func(o PodLogsOptions) PodLogsOptions {
				o.Image = "example.com/img:v2"
				return o
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.objects...)
			opts := snapshotOpts()
			if tt.opts != nil {
				opts = tt.opts(opts)
			}

			out := &bytes.Buffer{}
			err := snapshotPodLogs(context.Background(), client, "default", opts, out)

			if !errors.Is(err, ErrNoMatchingPods) {
				t.Errorf("expected ErrNoMatchingPods, got %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("expected no output, got %q", out.String())
			}
		})
	}
}

// TestSnapshotPodLogs_SinglePod ensures the logs of a matching pod are written
// verbatim, terminated, and unprefixed.
func TestSnapshotPodLogs_SinglePod(t *testing.T) {
	client := fake.NewClientset(runningPod("pod-a", "example.com/img:v1"))

	out := &bytes.Buffer{}
	if err := snapshotPodLogs(context.Background(), client, "default", snapshotOpts(), out); err != nil {
		t.Fatal(err)
	}

	// The fake logs are unterminated: a terminating newline is expected to be
	// supplied, but nothing else.
	if expected := fakeLogs + "\n"; out.String() != expected {
		t.Errorf("expected %q, got %q", expected, out.String())
	}
}

// TestSnapshotPodLogs_MultiplePods ensures the logs of several matching pods
// are attributed, ordered deterministically, and not run together.
func TestSnapshotPodLogs_MultiplePods(t *testing.T) {
	client := fake.NewClientset(
		runningPod("pod-b", "example.com/img:v1"),
		runningPod("pod-a", "example.com/img:v1"),
	)

	out := &bytes.Buffer{}
	if err := snapshotPodLogs(context.Background(), client, "default", snapshotOpts(), out); err != nil {
		t.Fatal(err)
	}

	expected := "[pod/pod-a] " + fakeLogs + "\n[pod/pod-b] " + fakeLogs + "\n"
	if out.String() != expected {
		t.Errorf("expected %q, got %q", expected, out.String())
	}
}

// TestSnapshotPodLogs_ImageFilter ensures that, when an image is given, only
// the logs of pods running it are gathered.
func TestSnapshotPodLogs_ImageFilter(t *testing.T) {
	client := fake.NewClientset(
		runningPod("pod-a", "example.com/img:v1"),
		runningPod("pod-b", "example.com/img:v2"),
	)

	opts := snapshotOpts()
	opts.Image = "example.com/img:v2"

	out := &bytes.Buffer{}
	if err := snapshotPodLogs(context.Background(), client, "default", opts, out); err != nil {
		t.Fatal(err)
	}

	// Only one pod matches, so its logs are written unprefixed.
	if expected := fakeLogs + "\n"; out.String() != expected {
		t.Errorf("expected only the logs of pod-b, got %q", out.String())
	}
}

// TestSnapshotPodLogs_Options ensures the options which bound the logs
// returned are passed to the API, and that a snapshot does not follow.
func TestSnapshotPodLogs_Options(t *testing.T) {
	client := fake.NewClientset(runningPod("pod-a", "example.com/img:v1"))

	since := time.Now().Add(-5 * time.Minute)
	tail := int64(20)
	opts := snapshotOpts()
	opts.Since = &since
	opts.TailLines = &tail

	if err := snapshotPodLogs(context.Background(), client, "default", opts, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	podLogOpts := requestedPodLogOptions(t, client)
	if podLogOpts.Follow {
		t.Error("expected a snapshot to not follow")
	}
	if podLogOpts.Container != "user-container" {
		t.Errorf("expected the user container, got %q", podLogOpts.Container)
	}
	if podLogOpts.TailLines == nil || *podLogOpts.TailLines != tail {
		t.Errorf("expected tail lines %v, got %v", tail, podLogOpts.TailLines)
	}
	// metav1.Time carries second precision.
	if podLogOpts.SinceTime == nil || podLogOpts.SinceTime.Time.Sub(since).Abs() >= time.Second {
		t.Errorf("expected since time %v, got %v", since, podLogOpts.SinceTime)
	}
}

// TestSnapshotPodLogs_PartialFailure ensures that a pod whose logs cannot be
// written does not discard the logs already gathered from other pods, and that
// the failure is reported as partial rather than as a plain error.
func TestSnapshotPodLogs_PartialFailure(t *testing.T) {
	client := fake.NewClientset(
		runningPod("pod-a", "example.com/img:v1"),
		runningPod("pod-b", "example.com/img:v1"),
	)

	// Fails once the first pod's logs have been written: a prefix, the logs
	// themselves and the terminating newline.
	out := &failingWriter{failAfter: 3}

	err := snapshotPodLogs(context.Background(), client, "default", snapshotOpts(), out)

	var partial *PartialLogsError
	if !errors.As(err, &partial) {
		t.Fatalf("expected a PartialLogsError, got %v", err)
	}
	if expected := "[pod/pod-a] " + fakeLogs + "\n"; out.written.String() != expected {
		t.Errorf("expected the logs gathered before the failure to be kept, got %q", out.written.String())
	}
}

// TestSnapshotPodLogs_TotalFailure ensures that a failure to gather the logs of
// every matching pod is reported as an error, not as a partial success.
func TestSnapshotPodLogs_TotalFailure(t *testing.T) {
	client := fake.NewClientset(runningPod("pod-a", "example.com/img:v1"))

	err := snapshotPodLogs(context.Background(), client, "default", snapshotOpts(), &failingWriter{})

	var partial *PartialLogsError
	if err == nil || errors.As(err, &partial) {
		t.Fatalf("expected a plain error, got %v", err)
	}
}

// TestSnapshotPodLogs_ListError ensures a failure to list pods is reported
// rather than presented as an absence of pods.
func TestSnapshotPodLogs_ListError(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	err := snapshotPodLogs(context.Background(), client, "default", snapshotOpts(), &bytes.Buffer{})
	if err == nil || errors.Is(err, ErrNoMatchingPods) {
		t.Errorf("expected the listing error to be reported, got %v", err)
	}
}

// TestLinePrefixer ensures lines are prefixed and terminated regardless of how
// the logs are chunked, as a pod's logs arrive in arbitrarily sized parts and
// its last line need not be terminated.
func TestLinePrefixer(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		chunks   []string
		expected string
	}{
		{
			name:     "unterminated last line",
			chunks:   []string{"a\nb"},
			expected: "a\nb\n",
		},
		{
			name:     "line split across chunks",
			prefix:   "[p] ",
			chunks:   []string{"hel", "lo\nwor", "ld\n"},
			expected: "[p] hello\n[p] world\n",
		},
		{
			name:     "empty lines are prefixed",
			prefix:   "[p] ",
			chunks:   []string{"\n\n"},
			expected: "[p] \n[p] \n",
		},
		{
			name:     "no output",
			prefix:   "[p] ",
			chunks:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			l := &linePrefixer{out: out, prefix: []byte(tt.prefix)}
			for _, chunk := range tt.chunks {
				n, err := l.Write([]byte(chunk))
				if err != nil {
					t.Fatal(err)
				}
				// io.Copy requires the count to be that of the bytes given,
				// excluding any prefix written.
				if n != len(chunk) {
					t.Fatalf("expected %v bytes written, got %v", len(chunk), n)
				}
			}
			if err := l.terminate(); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, out.String())
			}
		})
	}
}

// requestedPodLogOptions returns the options of the single log request the
// client received.
func requestedPodLogOptions(t *testing.T, client kubernetes.Interface) corev1.PodLogOptions {
	t.Helper()
	f, ok := client.(*fake.Clientset)
	if !ok {
		t.Fatal("expected a fake clientset")
	}
	for _, action := range f.Actions() {
		if action.GetVerb() != "get" || action.GetSubresource() != "log" {
			continue
		}
		generic, ok := action.(k8stesting.GenericAction)
		if !ok {
			t.Fatalf("expected a generic action, got %T", action)
		}
		opts, ok := generic.GetValue().(*corev1.PodLogOptions)
		if !ok {
			t.Fatalf("expected pod log options, got %T", generic.GetValue())
		}
		return *opts
	}
	t.Fatal("no logs were requested")
	return corev1.PodLogOptions{}
}

// failingWriter writes until the given number of writes have been made, and
// fails from then on, as stdout does when its reader goes away.
type failingWriter struct {
	failAfter int
	writes    int
	written   bytes.Buffer
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("broken pipe")
	}
	w.writes++
	return w.written.Write(p)
}
