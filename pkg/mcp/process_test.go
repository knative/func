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

	r.activate("/a/b", 111, stop, nil)

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
	r.activate("/a", 1, func() error { stoppedA = true; return nil }, nil)

	if err := r.reserve("/b"); err != nil {
		t.Fatal(err)
	}
	r.activate("/b", 2, func() error { stoppedB = true; return nil }, nil)

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

	pid, host, port, stop, _, err := starter.Start(ctx, "run")
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

	_, _, _, _, _, err := starter.Start(ctx, "run")
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

	_, _, _, _, _, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
}

// TestRunRegistry_ActivateDropsEntryOnExit verifies that an active entry is
// removed from the registry when its process exits on its own, so the path
// can be reserved again without a run_stop call.
func TestRunRegistry_ActivateDropsEntryOnExit(t *testing.T) {
	r := newRunRegistry()
	exited := make(chan struct{})

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { return nil }, exited)
	if _, ok := r.get("/a"); !ok {
		t.Fatal("expected entry to be active before the process exits")
	}

	close(exited)
	waitFor(t, "entry to be dropped after process exit", func() bool {
		_, ok := r.get("/a")
		return !ok
	})

	if err := r.reserve("/a"); err != nil {
		t.Fatalf("expected path to be reservable after the process exited, got: %v", err)
	}
}

// TestRunRegistry_ActivateAlreadyExited covers the window between Start
// returning and activate being called: if the process exited in that window
// the channel is already closed, and the entry must still be dropped.
func TestRunRegistry_ActivateAlreadyExited(t *testing.T) {
	r := newRunRegistry()
	exited := make(chan struct{})
	close(exited)

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { return nil }, exited)

	waitFor(t, "entry to be dropped for an already-exited process", func() bool {
		_, ok := r.get("/a")
		return !ok
	})
}

// TestRunRegistry_ExitDoesNotDropNewerEntry verifies that the exit of an
// old process cannot remove a newer entry that has since claimed the same
// path (for example after run_stop removed the old one and a new run
// started).
func TestRunRegistry_ExitDoesNotDropNewerEntry(t *testing.T) {
	r := newRunRegistry()
	oldExited := make(chan struct{})

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { return nil }, oldExited)
	oldEntry, _ := r.get("/a")

	// The old entry is replaced by a new run before the old exit is seen.
	r.remove("/a")
	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 2, func() error { return nil }, nil)

	// Neither the exit notification nor a late removeEntry for the old
	// entry may touch the new one.
	close(oldExited)
	r.removeEntry("/a", oldEntry)
	time.Sleep(50 * time.Millisecond) // let the exit watcher run

	entry, ok := r.get("/a")
	if !ok || entry.pid != 2 {
		t.Fatalf("expected the newer entry (pid 2) to remain, got %+v (ok=%v)", entry, ok)
	}
}

// TestRunRegistry_ExitAfterStopAll verifies that the exit watcher is a
// no-op once stopAll has already cleared the registry.
func TestRunRegistry_ExitAfterStopAll(t *testing.T) {
	r := newRunRegistry()
	exited := make(chan struct{})

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { close(exited); return nil }, exited)

	r.stopAll() // stop closes exited, waking the watcher
	time.Sleep(50 * time.Millisecond)

	if _, ok := r.get("/a"); ok {
		t.Fatal("expected registry to stay empty")
	}
	if err := r.reserve("/a"); err != nil {
		t.Fatalf("unexpected error reserving after stopAll: %v", err)
	}
}

// waitFor polls cond until it returns true or a timeout elapses.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestDefaultProcessStarter_ExitedClosesOnSelfExit verifies that the exited
// channel returned by Start is closed when the process terminates on its
// own after having become ready, and that stop afterwards remains a safe
// no-op.
func TestDefaultProcessStarter_ExitedClosesOnSelfExit(t *testing.T) {
	script := writeTestScript(t, 0)
	body := "#!/bin/sh\necho '{\"host\":\"127.0.0.1\",\"port\":\"9999\"}'\nsleep 0.3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, stop, exited, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("expected exited to be closed after the process exited on its own")
	}
	if err = stop(); err != nil {
		t.Fatalf("unexpected error from stop after self-exit: %v", err)
	}
}

// TestDefaultProcessStarter_ExitedClosesOnStop verifies that stopping the
// process also closes the exited channel.
func TestDefaultProcessStarter_ExitedClosesOnStop(t *testing.T) {
	script := writeTestScript(t, 0, `{"host":"127.0.0.1","port":"9999"}`)
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, stop, exited, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-exited:
		t.Fatal("exited must not be closed while the process is running")
	default:
	}

	if err = stop(); err != nil {
		t.Fatalf("unexpected error from stop: %v", err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("expected exited to be closed once stop returned")
	}
}
