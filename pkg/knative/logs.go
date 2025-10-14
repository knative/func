package knative

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"knative.dev/func/pkg/k8s"
)

// GetKServiceLogs will get logs of Knative service.
//
// It will do so by gathering logs of user-container of all affiliated pods.
// In addition, filtering on image can be done so only logs for given image are logged.
// The image must be the digest format since pods of Knative service use it.
//
// This function runs as long as the passed context is active (i.e. it is required cancel the context to stop log gathering).
func GetKServiceLogs(ctx context.Context, namespace, kServiceName, image string, since *time.Time, out io.Writer) error {
	selector := fmt.Sprintf("serving.knative.dev/service=%s", kServiceName)
	return k8s.GetPodLogsBySelector(ctx, namespace, selector, "user-container", image, since, out)
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
