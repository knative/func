package mock

import (
	"context"
	"sync"
	"time"

	fn "knative.dev/func/pkg/functions"
)

// Runner runs a function in a separate process, canceling it on context.Cancel.
// Immediately returned is the port of the running function.
type Runner struct {
	RunInvoked    bool
	RootRequested string
	RunFn         func(context.Context, fn.Function, string, time.Duration) (*fn.Job, error)
	sync.Mutex
}

func NewRunner() *Runner {
	return &Runner{
		RunFn: func(ctx context.Context, f fn.Function, addr string, t time.Duration) (*fn.Job, error) {
			errs := make(chan error, 1)
			stop := func() error { return nil }
			return fn.NewJob(f, "127.0.0.1", "8080", errs, stop, false)
		},
	}
}

func (r *Runner) Run(ctx context.Context, f fn.Function, addr string, t time.Duration) (*fn.Job, error) {
	r.Lock()
	defer r.Unlock()
	r.RunInvoked = true
	r.RootRequested = f.Root

	return r.RunFn(ctx, f, addr, t)
}
