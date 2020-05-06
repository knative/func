package mock

type Runner struct {
	RunInvoked    bool
	RootRequested string
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(root string) error {
	r.RunInvoked = true
	r.RootRequested = root
	return nil
}
