package mock

import bosonFunc "github.com/boson-project/func"

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(f bosonFunc.Function) error {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return nil
}
