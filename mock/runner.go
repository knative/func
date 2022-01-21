package mock

import (
	"context"
	"sync"

	fn "knative.dev/kn-plugin-func"
)

// Runner runs a Function in a separate process, canceling it on context.Cancel.
// Immediately returned is the port of the running Function.
type Runner struct {
	RunInvoked    bool
	RootRequested string
	RunFn         func(context.Context, fn.Function) (*fn.Job, error)
	sync.Mutex
}

func NewRunner() *Runner {
	return &Runner{
		RunFn: func(ctx context.Context, f fn.Function) (*fn.Job, error) {
			errs := make(chan error, 1)
			stop := func() {}
			return fn.NewJob(f, "8080", errs, stop)
		},
	}
}

func (r *Runner) Run(ctx context.Context, f fn.Function) (*fn.Job, error) {
	r.Lock()
	defer r.Unlock()
	r.RunInvoked = true
	r.RootRequested = f.Root

	return r.RunFn(ctx, f)
}
