package cmd

import (
	"errors"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestErrNotInitializedErrorStrings validates that ErrNotInitialized.Error()
// returns command-specific help text for the supported commands.
func TestErrNotInitializedErrorStrings(t *testing.T) {
	err := NewErrNotInitializedFromPath("/tmp/test", "invoke")
	if !strings.Contains(err.Error(), "No function found in provided path") {
		t.Errorf("expected invoke guidance, got: %v", err.Error())
	}

	errDesc := NewErrNotInitializedFromPath("/tmp/test", "describe")
	if !strings.Contains(errDesc.Error(), "No function found in provided path") {
		t.Errorf("expected describe guidance, got: %v", errDesc.Error())
	}

	errSub := NewErrNotInitializedFromPath("/tmp/test", "subscribe")
	if !strings.Contains(errSub.Error(), "func subscribe --filter") {
		t.Errorf("expected subscribe guidance, got: %v", errSub.Error())
	}

	errCfg := NewErrNotInitializedFromPath("/tmp/test", "config")
	if !strings.Contains(errCfg.Error(), "func config --help") {
		t.Errorf("expected config guidance, got: %v", errCfg.Error())
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

	// generic error passes through
	genericErr := errors.New("some other error")
	if wrapInvokeError(genericErr) != genericErr {
		t.Error("expected generic error to pass through unchanged")
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
