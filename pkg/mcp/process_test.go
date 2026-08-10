package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRunRegistry_ReserveActivateRemove verifies the basic lifecycle of a
// runRegistry entry, including rejection of a duplicate reserve for the same
// path both before and after activation.
func TestRunRegistry_ReserveActivateRemove(t *testing.T) {
	r := newRunRegistry()
	stopped := false
	stop := func() error { stopped = true; return nil }

	if _, ok := r.get("/a/b"); ok {
		t.Fatal("expected no entry before reserve")
	}

	if err := r.reserve("/a/b"); err != nil {
		t.Fatalf("unexpected error reserving: %v", err)
	}

	// A pending (reserved but not yet activated) entry is not "active".
	if _, ok := r.get("/a/b"); ok {
		t.Fatal("expected no active entry while still pending")
	}

	// A second reserve while pending must be rejected.
	if err := r.reserve("/a/b"); err == nil {
		t.Fatal("expected error reserving an already-pending path")
	}

	r.activate("/a/b", 111, stop)

	entry, ok := r.get("/a/b")
	if !ok {
		t.Fatal("expected entry after activate")
	}
	if entry.pid != 111 {
		t.Fatalf("expected pid 111, got %d", entry.pid)
	}

	// A reserve for an already-active path must be rejected.
	if err := r.reserve("/a/b"); err == nil {
		t.Fatal("expected error reserving an already-active path")
	}

	r.remove("/a/b")
	if _, ok := r.get("/a/b"); ok {
		t.Fatal("expected no entry after remove")
	}
	if err := entry.stop(); err != nil {
		t.Fatalf("unexpected error calling stop: %v", err)
	}
	if !stopped {
		t.Fatal("expected stop to have been invoked")
	}
}

// TestRunRegistry_StopAll verifies that stopAll stops every active run and
// clears the registry.
func TestRunRegistry_StopAll(t *testing.T) {
	r := newRunRegistry()
	var stoppedA, stoppedB bool

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { stoppedA = true; return nil })

	if err := r.reserve("/b"); err != nil {
		t.Fatal(err)
	}
	r.activate("/b", 2, func() error { stoppedB = true; return nil })

	r.stopAll()

	if !stoppedA || !stoppedB {
		t.Fatalf("expected both runs to be stopped, got a=%v b=%v", stoppedA, stoppedB)
	}
	if _, ok := r.get("/a"); ok {
		t.Fatal("expected registry to be cleared after stopAll")
	}
	if _, ok := r.get("/b"); ok {
		t.Fatal("expected registry to be cleared after stopAll")
	}
}

// TestResolveRunPath verifies that a required absolute path is accepted
// as-is (cleaned), and that a relative path is rejected rather than
// resolved against the server process's working directory.
func TestResolveRunPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(wd, "myfunc")

	got, err := resolveRunPath(abs)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("expected %q, got %q", abs, got)
	}

	if _, err = resolveRunPath(""); err == nil {
		t.Fatal("expected error for empty path")
	}

	if _, err = resolveRunPath("."); err == nil {
		t.Fatal("expected error for relative path")
	}

	if _, err = resolveRunPath("myfunc"); err == nil {
		t.Fatal("expected error for relative path")
	}
}

// writeTestScript writes an executable shell script to a temp file that:
//   - traps SIGTERM to exit cleanly (mimicking func run's graceful shutdown)
//   - optionally sleeps before printing anything (to simulate slow startup)
//   - prints the given stdout line(s)
//   - then idles until signaled or killed
func writeTestScript(t *testing.T, preSleep time.Duration, stdoutLines ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test script requires a POSIX shell")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-func.sh")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("trap 'exit 0' TERM\n")
	if preSleep > 0 {
		fmt.Fprintf(&b, "sleep %f\n", preSleep.Seconds())
	}
	for _, line := range stdoutLines {
		fmt.Fprintf(&b, "echo '%s'\n", line)
	}
	b.WriteString("while true; do sleep 1; done\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestStarter(t *testing.T, script string) defaultProcessStarter {
	t.Helper()
	return defaultProcessStarter{s: &Server{prefix: script}}
}

// TestDefaultProcessStarter_Ready verifies that Start waits for the
// readiness line, then returns pid/host/port, and that the returned stop
// function terminates the process via SIGTERM.
func TestDefaultProcessStarter_Ready(t *testing.T) {
	script := writeTestScript(t, 0, `{"host":"127.0.0.1","port":"9999"}`)
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	pid, host, port, stop, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive pid, got %d", pid)
	}
	if host != "127.0.0.1" || port != "9999" {
		t.Fatalf("expected 127.0.0.1:9999, got %s:%s", host, port)
	}

	if err = stop(); err != nil {
		t.Fatalf("unexpected error from stop: %v", err)
	}
	// stop must be idempotent.
	if err = stop(); err != nil {
		t.Fatalf("unexpected error from second stop call: %v", err)
	}
}

// TestDefaultProcessStarter_ExitsBeforeReady verifies that a process which
// exits without ever emitting a valid readiness line surfaces a clear error.
func TestDefaultProcessStarter_ExitsBeforeReady(t *testing.T) {
	script := writeTestScript(t, 0)
	// Override the idle loop with an immediate exit by writing our own
	// script body instead of relying on writeTestScript's infinite loop.
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'not json'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, _, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected an error when the process exits before becoming ready")
	}
}

// TestDefaultProcessStarter_Timeout verifies that Start gives up and returns
// an error if the process never becomes ready within the given context.
func TestDefaultProcessStarter_Timeout(t *testing.T) {
	script := writeTestScript(t, 3*time.Second, `{"host":"127.0.0.1","port":"9999"}`)
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	_, _, _, _, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
}
