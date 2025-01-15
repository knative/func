package cmd

import (
	"context"
	"fmt"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
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
			root := FromTempDirectory(t)

			runner := mock.NewRunner()
			if tt.runError != nil {
				runner.RunFn = func(context.Context, fn.Function, time.Duration) (*fn.Job, error) { return nil, tt.runError }
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

// TestRun_Images ensures that runnning 'func run' with --image
// (and additional flags) works as intended
func TestRun_Images(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		buildInvoked bool
		runInvoked   bool

		runError   error
		buildError error
	}{
		{
			name:         "image with digest",
			args:         []string{"--image", "exampleimage@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
			runInvoked:   true,
			buildInvoked: false,
		},
		{
			name:         "image with tag direct deploy",
			args:         []string{"--image", "username/exampleimage:latest", "--build=false"},
			runInvoked:   true,
			buildInvoked: false,
		},
		{
			name:         "digested image without container should fail",
			args:         []string{"--container=false", "--image", "exampleimage@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
			runInvoked:   false,
			buildInvoked: false,
			buildError:   fmt.Errorf("cannot use digested image with --container=false"),
		},
		{
			name:         "image should build even with tagged image given",
			args:         []string{"--image", "username/exampleimage:latest"},
			runInvoked:   true,
			buildInvoked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := FromTempDirectory(t)
			runner := mock.NewRunner()

			if tt.runError != nil {
				runner.RunFn = func(context.Context, fn.Function, time.Duration) (*fn.Job, error) { return nil, tt.runError }
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
			_, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
			if err != nil {
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

// TestRun_CorrectImage enusures that correct image gets passed through to the
// runner.
func TestRun_CorrectImage(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		args         []string
		buildInvoked bool
		expectError  bool
	}{
		{
			name:         "image with digest, auto build",
			args:         []string{"--image", "exampleimage@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
			image:        "exampleimage@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			buildInvoked: false,
		},
		{
			name:         "image with tag direct deploy",
			args:         []string{"--image", "username/exampleimage:latest", "--build=false"},
			image:        "username/exampleimage:latest",
			buildInvoked: false,
		},
		{
			name:         "digested image without container should fail",
			args:         []string{"--container=false", "--image", "exampleimage@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
			image:        "",
			buildInvoked: false,
			expectError:  true,
		},
		{
			name:         "image should build even with tagged image given",
			args:         []string{"--image", "username/exampleimage:latest"},
			image:        "username/exampleimage:latest",
			buildInvoked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := FromTempDirectory(t)
			runner := mock.NewRunner()

			runner.RunFn = func(_ context.Context, f fn.Function, _ time.Duration) (*fn.Job, error) {
				// TODO: add if for empty image? -- should fail beforehand
				if f.Build.Image != tt.image {
					return nil, fmt.Errorf("Expected image: %v but got: %v", tt.image, f.Build.Image)
				}
				errs := make(chan error, 1)
				stop := func() error { return nil }
				return fn.NewJob(f, "127.0.0.1", "8080", errs, stop, false)
			}

			builder := mock.NewBuilder()
			if tt.expectError {
				builder.BuildFn = func(f fn.Function) error { return fmt.Errorf("expected error") }
			}

			cmd := NewRunCmd(NewTestClient(
				fn.WithRunner(runner),
				fn.WithBuilder(builder),
				fn.WithRegistry("ghcr.com/reg"),
			))
			cmd.SetArgs(tt.args)

			// set test case's function instance
			_, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			runErrCh := make(chan error, 1)
			go func() {
				t0 := tt // capture tt into closure
				_, err := cmd.ExecuteContextC(ctx)
				if err != nil && t0.expectError {
					// This is an expected error, so simply continue execution ignoring
					// the error (send nil on the channel to release the parent routine
					runErrCh <- nil
					return
				} else if err != nil {
					runErrCh <- err // error not expected
					return
				}

				// No errors, but an error was expected:
				if t0.expectError {
					runErrCh <- fmt.Errorf("Expected error but got '%v'\n", err)
				}

				// Ensure invocations match expectations
				if builder.BuildInvoked != tt.buildInvoked {
					runErrCh <- fmt.Errorf("Function was expected to build is: %v but build execution was: %v", tt.buildInvoked, builder.BuildInvoked)
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

// TestRun_DirectOverride tests that an --image passed after a function has
// already been build, the given --image with digest will override built function
func TestRun_DirectOverride(t *testing.T) {
	const overrideImage = "registry/myrepo/myimage@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	root := FromTempDirectory(t)
	runner := mock.NewRunner()

	runner.RunFn = func(_ context.Context, f fn.Function, _ time.Duration) (*fn.Job, error) {
		if f.Build.Image != overrideImage {
			return nil, fmt.Errorf("Expected image to be overridden with '%v' but got: '%v'", overrideImage, f.Build.Image)
		}
		errs := make(chan error, 1)
		stop := func() error { return nil }
		return fn.NewJob(f, "127.0.0.1", "8080", errs, stop, false)
	}

	builder1 := mock.NewBuilder()

	// SETUP THE ENVIRONMENT & SITUATION
	// create function
	_, err := fn.New().Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	// build function
	cmdBuild := NewBuildCmd(NewTestClient(fn.WithBuilder(builder1), fn.WithRegistry("example.com/ns-to-override")))
	if err := cmdBuild.Execute(); err != nil {
		t.Fatal(err)
	}

	// fetch the functions state
	_, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// builder for 'func run' -- shall not be invoked
	builder2 := mock.NewBuilder()
	builder2.BuildFn = func(f fn.Function) error {
		return fmt.Errorf("should not be invoked")
	}

	// RUN THE ACTUAL TESTED COMMAND
	cmd := NewRunCmd(NewTestClient(
		fn.WithRunner(runner),
		fn.WithBuilder(builder2),
		fn.WithRegistry("ghcr.com/reg"),
	))
	cmd.SetArgs([]string{fmt.Sprintf("--image=%s", overrideImage)})

	// run function with above argument
	ctx, cancel := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		_, err := cmd.ExecuteContextC(ctx)
		if err != nil {
			runErrCh <- err // error was not expected
			return
		}

		// Ensure invocation doesnt happen for the second time as the image was
		// provided with a digest (should not build)
		if builder2.BuildInvoked {
			runErrCh <- fmt.Errorf("Function was not expected to build again but it did")
		}

		close(runErrCh) // release the waiting parent process
	}()
	cancel() // trigger the return of cmd.ExecuteContextC in the routine
	<-ctx.Done()
	if err := <-runErrCh; err != nil { // wait for completion of assertions
		t.Fatal(err)
	}
}
