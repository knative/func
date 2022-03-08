package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

func TestRun_Run(t *testing.T) {
	tests := []struct {
		name         string // name of the test
		desc         string // description of the test
		funcState    string // Function state, as described in func.yaml
		buildFlag    bool   // value to which the --build flag should be set
		buildError   error  // Set the builder to yield this error
		runError     error  // Set the runner to yield this error
		buildInvoked bool   // should Builder.Build be invoked?
		runInvoked   bool   // should Runner.Run be invoked?
	}{
		{
			name: "run when not building",
			desc: "Should run when build is not enabled",
			funcState: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:    false,
			buildInvoked: false,
			runInvoked:   true,
		},
		{
			name: "run and build",
			desc: "Should run and build when build is enabled and there is no image",
			funcState: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:    true,
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name: "skip rebuild",
			desc: "Built image doesn't get built again",
			// TODO: this might be improved by checking if the user provided
			// the --build=true flag, allowing an override to force rebuild.
			// This could be accomplished by adding a 'provideBuildFlag' struct
			// member.
			funcState: `name: test-func
runtime: go
image: exampleimage
created: 2009-11-10 23:00:00`,
			buildFlag:    true,
			buildInvoked: false,
			runInvoked:   true,
		},
		{
			name: "Build errors return",
			desc: "Errors building cause an immediate return with error",
			funcState: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:    true,
			buildError:   fmt.Errorf("generic build error"),
			buildInvoked: true,
			runInvoked:   false,
		},
	}
	for _, tt := range tests {
		// run as a sub-test
		t.Run(tt.name, func(t *testing.T) {
			defer fromTempDir(t)()

			runner := mock.NewRunner()
			if tt.runError != nil {
				runner.RunFn = func(context.Context, fn.Function) (*fn.Job, error) { return nil, tt.runError }
			}

			builder := mock.NewBuilder()
			if tt.buildError != nil {
				builder.BuildFn = func(f fn.Function) error { return tt.buildError }
			}

			// using a command whose client will be populated with mock
			// builder and mock runner, each of which may be set to error if the
			// test has an error defined.
			cmd := NewRunCmd(
				fn.WithRunner(runner),
				fn.WithBuilder(builder),
				fn.WithRegistry("ghcr.com/reg"),
			)

			// set test case's build
			viper.SetDefault("build", tt.buildFlag)

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcState), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			runErrCh := make(chan error, 1)
			go func() {
				t0 := tt // capture tt into closure
				_, err := cmd.ExecuteContextC(ctx)
				if err != nil && t0.buildError != nil {
					// This is an expected error, so simply continue execution ignoring
					// the error (send nil on the channel to release the parent routine
					runErrCh <- nil
					return
				} else if err != nil {
					runErrCh <- err // error not expected
					return
				}

				// No errors, but an error was expected:
				if t0.buildError != nil {
					runErrCh <- fmt.Errorf("Expected error: %v but got %v\n", t0.buildError, err)
				}

				// Ensure invocations match expectations
				if builder.BuildInvoked != tt.buildInvoked {
					runErrCh <- fmt.Errorf("Function was expected to build is: %v but build execution was: %v", tt.buildInvoked, builder.BuildInvoked)
				}
				if runner.RunInvoked != tt.runInvoked {
					runErrCh <- fmt.Errorf("Function was expected to run is: %v but run execution was: %v", tt.runInvoked, runner.RunInvoked)
				}

				close(runErrCh) // release the waiting parent process
			}()
			cancel() // trigger the return of cmd.ExecuteContextC in the routine
			<-ctx.Done()
			if err := <-runErrCh; err != nil { // wait for completion of assertions
				t.Fatal(err)
			}
		})
	}
}
