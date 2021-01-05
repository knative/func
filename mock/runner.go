package mock

import function "github.com/boson-project/func"

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(f function.Function) error {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return nil
}
