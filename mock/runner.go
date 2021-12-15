package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

// Runner runs a Function in a separate process, canceling it on context.Cancel.
// Immediately returned is the pid and port of the process started.
// If RunFn is overridden, it should not block as its pid and port need to be
// returned up the stack for the Client to be able to register the Function
// is now running.
type Runner struct {
	RunInvoked    bool
	RootRequested string
	RunFn         func(context.Context, fn.Function) (int, int, error)
}

func NewRunner() *Runner {
	return &Runner{
		RunFn: func(context.Context, fn.Function) (int, int, error) {
			return 0, 0, nil
		},
	}
}

func (r *Runner) Run(ctx context.Context, f fn.Function) (int, int, error) {
	r.RunInvoked = true
	r.RootRequested = f.Root

	var (
		pid  int
		port int
		err  error
	)

	// Run the Function
	// In this case a separate process is mocked using a goroutine which invokes
	// the RunFn.  RunFn can block or not, but if it blocks should
	started := make(chan bool, 1) // signal the "Function" is started
	go func() {
		pid, port, err = r.RunFn(ctx, f)
		started <- true
		<-ctx.Done()
	}()

	<-started // wait for "start" (pid and port populated)
	return pid, port, err

}
