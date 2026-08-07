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

// TestRunRegistry_AddGetRemove verifies the basic lifecycle of a runRegistry
// entry, including rejection of a duplicate add for the same path.
func TestRunRegistry_AddGetRemove(t *testing.T) {
	r := newRunRegistry()
	stopped := false
	stop := func() error { stopped = true; return nil }

	if _, ok := r.get("/a/b"); ok {
		t.Fatal("expected no entry before add")
	}

	if err := r.add("/a/b", 111, stop); err != nil {
		t.Fatalf("unexpected error adding: %v", err)
	}

	entry, ok := r.get("/a/b")
	if !ok {
		t.Fatal("expected entry after add")
	}
	if entry.pid != 111 {
		t.Fatalf("expected pid 111, got %d", entry.pid)
	}

	// Duplicate add for the same path must be rejected.
	if err := r.add("/a/b", 222, func() error { return nil }); err == nil {
		t.Fatal("expected error adding duplicate path")
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

// TestResolveRunPath verifies path resolution defaults to the working
// directory when omitted, and resolves relative paths to absolute ones.
func TestResolveRunPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveRunPath(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != wd {
		t.Fatalf("expected %q, got %q", wd, got)
	}

	empty := ""
	got, err = resolveRunPath(&empty)
	if err != nil {
		t.Fatal(err)
	}
	if got != wd {
		t.Fatalf("expected %q, got %q", wd, got)
	}

	rel := "."
	got, err = resolveRunPath(&rel)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(rel)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
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
		b.WriteString(fmt.Sprintf("sleep %f\n", preSleep.Seconds()))
	}
	for _, line := range stdoutLines {
		b.WriteString(fmt.Sprintf("echo '%s'\n", line))
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
	runStopGrace = 200 * time.Millisecond
	defer func() { runStopGrace = 10 * time.Second }()

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
	runStopGrace = 200 * time.Millisecond
	defer func() { runStopGrace = 10 * time.Second }()

	script := writeTestScript(t, 3*time.Second, `{"host":"127.0.0.1","port":"9999"}`)
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	_, _, _, _, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
}
