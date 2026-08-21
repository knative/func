package knative

import (
	"context"
	"fmt"
	"io"
	"time"

	"knative.dev/func/pkg/k8s"
)

// LogsOptions are the parameters of GetKServiceLogs.
type LogsOptions struct {
	// Name of the Knative service whose logs are gathered.
	Name string

	// Namespace of the Knative service.  An empty value resolves to the
	// currently active namespace.
	Namespace string

	// Image, when non-empty, restricts gathering to pods running this exact
	// image, which must be in digest format since pods of a Knative service
	// use it.  Useful to isolate the logs of a single revision.
	Image string

	// Since, when non-nil, is the time from which logs are returned.
	Since *time.Time

	// TailLines, when non-nil, limits the output to the last N lines per pod.
	TailLines *int64

	// Follow streams logs until the context is cancelled.  When false, the
	// logs currently available are written and the call returns.
	Follow bool
}

// GetKServiceLogs will get logs of Knative service.
//
// It will do so by gathering logs of user-container of all affiliated pods.
//
// When opts.Follow is set, this function runs as long as the passed context is
// active (i.e. it is required to cancel the context to stop log gathering).
// Otherwise a snapshot of the currently available logs is written and the
// function returns, with k8s.ErrNoMatchingPods if the service currently has no
// pods whose logs can be read.
func GetKServiceLogs(ctx context.Context, opts LogsOptions, out io.Writer) error {
	return k8s.GetPodLogsBySelector(ctx, k8s.PodLogsOptions{
		Namespace:     opts.Namespace,
		LabelSelector: fmt.Sprintf("serving.knative.dev/service=%s", opts.Name),
		Container:     "user-container",
		Image:         opts.Image,
		Since:         opts.Since,
		TailLines:     opts.TailLines,
		Follow:        opts.Follow,
	}, out)
}
