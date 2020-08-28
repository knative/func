package mock

import "github.com/boson-project/faas"

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(f faas.Function) error {
	r.RunInvoked = true
	r.RootRequested = f.Root
	return nil
}
