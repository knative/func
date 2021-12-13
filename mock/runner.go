package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Runner struct {
	RunInvoked    bool
	RootRequested string
	RunFn         func(context.Context, fn.Function) (int, int, error)

	StopInvoked bool
	StopFn      func(context.Context, fn.Function) error
}

func NewRunner() *Runner {
	return &Runner{
		RunFn:  func(context.Context, fn.Function) (int, int, error) { return 0, 0, nil },
		StopFn: func(context.Context, fn.Function) error { return nil },
	}
}

func (r *Runner) Run(ctx context.Context, f fn.Function) (int, int, error) {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return r.RunFn(ctx, f)
}

func (r *Runner) Stop(ctx context.Context, f fn.Function) error {
	r.StopInvoked = true
	return r.StopFn(ctx, f)
}
