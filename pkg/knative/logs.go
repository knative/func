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

	// Follow keeps streaming the logs until the context is cancelled.
	Follow bool
}

// GetKServiceLogs will get logs of Knative service.
//
// It will do so by gathering logs of user-container of all affiliated pods.
//
// This function runs as long as the passed context is active (i.e. it is
// required to cancel the context to stop log gathering).
func GetKServiceLogs(ctx context.Context, opts LogsOptions, out io.Writer) error {
	return k8s.GetPodLogsBySelector(ctx, k8s.PodLogsOptions{
		Namespace:     opts.Namespace,
		LabelSelector: fmt.Sprintf("serving.knative.dev/service=%s", opts.Name),
		Container:     "user-container",
		Image:         opts.Image,
		Since:         opts.Since,
		Follow:        opts.Follow,
	}, out)
}
