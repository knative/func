package mock

import (
	"context"
	fn "knative.dev/kn-plugin-func"
)

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, f fn.Function) error {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return nil
}
