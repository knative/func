package mock

import (
	"context"
	bosonFunc "github.com/boson-project/func"
)

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, f bosonFunc.Function) error {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return nil
}
