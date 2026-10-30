package cmd

import (
	"errors"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestErrNotInitializedErrorStrings validates that ErrNotInitialized.Error()
// returns command-specific help text for the supported commands.
func TestErrNotInitializedErrorStrings(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{
			cmd: "invoke",
			want: []string{
				"'/tmp/test' does not contain an initialized function",
				"func invoke",
				"Or if you have an existing function",
				"path/to/your/function",
				"run 'func invoke --help'",
			},
		},
		{
			cmd: "describe",
			want: []string{
				"func describe",
				"Or if you have an existing deployed function",
				"run 'func describe --help'",
			},
		},
		{
			cmd: "subscribe",
			want: []string{
				"func subscribe --filter type=example",
				"Or if you have an existing function",
				"run 'func subscribe --help'",
			},
		},
		{
			cmd: "config",
			want: []string{
				"func config",
				"Or if you have an existing function",
				"run 'func config --help'",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			msg := NewErrNotInitializedFromPath("/tmp/test", tc.cmd).Error()
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("expected %q in:\n%s", want, msg)
				}
			}
			if strings.Contains(msg, "Of if") {
				t.Errorf("help text has the %q typo:\n%s", "Of if", msg)
			}
			if strings.Contains(msg, "path/to/you/function") {
				t.Errorf("help text has a truncated path:\n%s", msg)
			}
		})
	}
}

// TestWrapDescribeError directly calls the wrapper to satisfy Codecov patch coverage.
func TestWrapDescribeError(t *testing.T) {
	// nil passthrough
	if err := wrapDescribeError(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// core ErrNotInitialized gets promoted to CLI type
	coreErr := fn.NewErrNotInitialized("/tmp/test")
	wrapped := wrapDescribeError(coreErr)
	var cliNotInit *ErrNotInitialized
	if !errors.As(wrapped, &cliNotInit) {
		t.Fatalf("expected *ErrNotInitialized, got %T: %v", wrapped, wrapped)
	}
	if cliNotInit.Cmd != "describe" {
		t.Fatalf("expected Cmd 'describe', got '%v'", cliNotInit.Cmd)
	}

	// already-wrapped CLI error passes through unchanged (no double-wrap)
	if wrapDescribeError(wrapped) != wrapped {
		t.Error("expected already-wrapped error to pass through unchanged")
	}

	// generic error passes through
	genericErr := errors.New("some other error")
	if wrapDescribeError(genericErr) != genericErr {
		t.Error("expected generic error to pass through unchanged")
	}

	// library cluster sentinel is wrapped, and still unwraps to that sentinel
	clusterWrapped := wrapDescribeError(fn.ErrClusterNotAccessible)
	var cluster *ErrDescribeClusterConnection
	if !errors.As(clusterWrapped, &cluster) {
		t.Fatalf("expected *ErrDescribeClusterConnection, got %T: %v", clusterWrapped, clusterWrapped)
	}
	if !errors.Is(clusterWrapped, fn.ErrClusterNotAccessible) {
		t.Fatal("expected wrapped cluster error to unwrap to fn.ErrClusterNotAccessible")
	}
	if !strings.Contains(clusterWrapped.Error(), "Cannot connect to Knative cluster") {
		t.Fatalf("expected cluster guidance, got: %v", clusterWrapped)
	}
}

// TestWrapInvokeError directly calls the wrapper to satisfy Codecov patch coverage.
func TestWrapInvokeError(t *testing.T) {
	// nil passthrough
	if err := wrapInvokeError(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// core ErrNotInitialized gets promoted to CLI type
	coreErr := fn.NewErrNotInitialized("/tmp/test")
	wrapped := wrapInvokeError(coreErr)
	var cliNotInit *ErrNotInitialized
	if !errors.As(wrapped, &cliNotInit) {
		t.Fatalf("expected *ErrNotInitialized, got %T: %v", wrapped, wrapped)
	}
	if cliNotInit.Cmd != "invoke" {
		t.Fatalf("expected Cmd 'invoke', got '%v'", cliNotInit.Cmd)
	}

	// already-wrapped CLI error passes through unchanged (no double-wrap)
	if wrapInvokeError(wrapped) != wrapped {
		t.Error("expected already-wrapped error to pass through unchanged")
	}

	// generic error passes through
	genericErr := errors.New("some other error")
	if wrapInvokeError(genericErr) != genericErr {
		t.Error("expected generic error to pass through unchanged")
	}

	// library not-running sentinel is wrapped, and still unwraps to that sentinel
	notRunning := wrapInvokeError(fn.ErrNotRunning)
	var running *ErrInvokeNotRunning
	if !errors.As(notRunning, &running) {
		t.Fatalf("expected *ErrInvokeNotRunning, got %T: %v", notRunning, notRunning)
	}
	if !errors.Is(notRunning, fn.ErrNotRunning) {
		t.Fatal("expected wrapped not-running error to unwrap to fn.ErrNotRunning")
	}
	if !strings.Contains(notRunning.Error(), "No running function instance found to invoke") {
		t.Fatalf("expected not-running guidance, got: %v", notRunning)
	}
}

// TestWrapSubscribeError directly calls the wrapper to satisfy Codecov patch coverage.
// Without this, wrapSubscribeError sits at 0% coverage and Codecov blocks the PR.
func TestWrapSubscribeError(t *testing.T) {
	//nil passthrough
	if err := wrapSubscribeError(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	//core ErrNotInitialied gets promoted to CLI type
	coreErr := fn.NewErrNotInitialized("/tmp/test")
	wrapped := wrapSubscribeError(coreErr)
	var cliNotInit *ErrNotInitialized
	if !errors.As(wrapped, &cliNotInit) {
		t.Fatalf("expected *ErrNotInitialized, got %T: %v", wrapped, wrapped)
	}
	if cliNotInit.Cmd != "subscribe" {
		t.Fatalf("expected Cmd 'subscribe', got '%v'", cliNotInit.Cmd)
	}

	//already-wrapped CLI error passes through unchanged (no double-wrap)
	if wrapSubscribeError(wrapped) != wrapped {
		t.Error("expected already-wrapped error to pass through unchanged")
	}

	// generic error passes through
	genericErr := errors.New("some other error")
	if wrapSubscribeError(genericErr) != genericErr {
		t.Error("expected generic error to pass through unchanged")
	}
}

// TestWrapConfigError directly calls the wrapper to satisfy Codecov patch coverage.
func TestWrapConfigError(t *testing.T) {
	// nil passthrough
	if err := wrapConfigError(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// core ErrNotInitialized gets promoted to CLI type
	coreErr := fn.NewErrNotInitialized("/tmp/test")
	wrapped := wrapConfigError(coreErr)
	var cliNotInit *ErrNotInitialized
	if !errors.As(wrapped, &cliNotInit) {
		t.Fatalf("expected *ErrNotInitialized, got %T: %v", wrapped, wrapped)
	}
	if cliNotInit.Cmd != "config" {
		t.Fatalf("expected Cmd 'config', got '%v'", cliNotInit.Cmd)
	}

	// already-wrapped CLI error passes through unchanged (no double-wrap)
	if wrapConfigError(wrapped) != wrapped {
		t.Error("expected already-wrapped error to pass through unchanged")
	}

	// generic error passes through
	genericErr := errors.New("some other error")
	if wrapConfigError(genericErr) != genericErr {
		t.Error("expected generic error to pass through unchanged")
	}
}

// TestConfigVolumes_WrapsNotInitialized runs the config command path, so a
// loader ErrNotInitialized is promoted rather than only the wrapper being called.
func TestConfigVolumes_WrapsNotInitialized(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := NewConfigVolumesCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when listing volumes from an empty directory")
	}

	var cliNotInit *ErrNotInitialized
	if !errors.As(err, &cliNotInit) {
		t.Fatalf("expected *ErrNotInitialized, got %T: %v", err, err)
	}
	if cliNotInit.Cmd != "config" {
		t.Fatalf("expected Cmd 'config', got %q", cliNotInit.Cmd)
	}
	if !strings.Contains(err.Error(), "run 'func config --help'") {
		t.Fatalf("expected config guidance, got: %v", err)
	}

	var core *fn.ErrNotInitialized
	if !errors.As(err, &core) {
		t.Fatal("expected CLI error to unwrap to fn.ErrNotInitialized")
	}
}
