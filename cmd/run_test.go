package cmd

import (
	"context"
	"fmt"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

func TestRun_Run(t *testing.T) {
	tests := []struct {
		name         string                              // name of the test
		desc         string                              // description of the test
		setup        func(fn.Function, *testing.T) error // Optionally mutate function
		args         []string                            // args for the test case
		buildError   error                               // Set the builder to yield this error
		runError     error                               // Set the runner to yield this error
		buildInvoked bool                                // should Builder.Build be invoked?
		runInvoked   bool                                // should Runner.Run be invoked?
	}{
		{
			name:         "run and build by default",
			desc:         "Should run and build when build flag is not specified",
			args:         []string{},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run and build flag",
			desc:         "Should run and build when build is merely provided (defaults to true on presence)",
			args:         []string{"--build"},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run and build",
			desc:         "Should run and build when build is specifically requested",
			args:         []string{"--build=true"},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run and build with builder pack",
			desc:         "Should run and build when build is specifically requested with builder pack",
			args:         []string{"--build=true", "--builder=pack"},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run and build with builder s2i",
			desc:         "Should run and build when build is specifically requested with builder s2i",
			args:         []string{"--build=true", "--builder=s2i"},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run and build with builder invalid",
			desc:         "Should run and build when build is specifically requested with builder invalid",
			args:         []string{"--build=true", "--builder=invalid"},
			buildError:   fmt.Errorf("\"invalid\" is not a known builder. Available builders are \"pack\" and \"s2i\""),
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "run without build when disabled",
			desc:         "Should run but not build when build is expressly disabled",
			args:         []string{"--build=false"}, // can be any truthy value: 0, 'false' etc.
			buildInvoked: false,
			runInvoked:   true,
		},
		{
			name:         "run and build on auto",
			desc:         "Should run and buil when build flag set to auto",
			args:         []string{"--build=auto"}, // can be any truthy value: 0, 'false' etc.
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name: "image existence builds",
			desc: "Should build when image tag exists",
			// The existence of an image tag value does not mean the function
			// is built; that is the purvew of the buld stamp staleness check.
			setup: func(f fn.Function, t *testing.T) error {
				f.Image = "exampleimage"
				return f.Write()
			},
			args:         []string{},
			buildInvoked: true,
			runInvoked:   true,
		},
		{
			name:         "Build errors return",
			desc:         "Errors building cause an immediate return with error",
			args:         []string{},
			buildError:   fmt.Errorf("generic build error"),
			buildInvoked: true,
			runInvoked:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fromTempDirectory(t)

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
			cmd := NewRunCmd(NewTestClient(
				fn.WithRunner(runner),
				fn.WithBuilder(builder),
				fn.WithRegistry("ghcr.com/reg"),
			))
			cmd.SetArgs(tt.args) // Do not use test command args

			// set test case's function instance
			f, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
			if err != nil {
				t.Fatal(err)
			}
			if tt.setup != nil {
				if err := tt.setup(f, t); err != nil {
					t.Fatal(err)
				}
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
