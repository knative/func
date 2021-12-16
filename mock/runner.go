package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

// Runner runs a Function in a separate process, canceling it on context.Cancel.
// Immediately returned is the port of the running Function.
type Runner struct {
	RunInvoked    bool
	RootRequested string
	RunFn         func(context.Context, fn.Function, chan error) (int, error)
}

func NewRunner() *Runner {
	return &Runner{
		RunFn: func(context.Context, fn.Function, chan error) (int, error) {
			return 0, nil
		},
	}
}

func (r *Runner) Run(ctx context.Context, f fn.Function, errCh chan error) (port int, err error) {
	r.RunInvoked = true
	r.RootRequested = f.Root

	// Run the Function
	// In this case a separate process is mocked using a goroutine which invokes
	// the RunFn.  RunFn can block or not, but if it blocks should
	started := make(chan bool, 1) // signal the "Function" is started
	go func() {
		port, err = r.RunFn(ctx, f, errCh)
		started <- true
		<-ctx.Done()
	}()
	<-started // wait for "start" (pid and port populated)
	return
}
