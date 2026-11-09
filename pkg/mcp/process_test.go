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

	r.activate("/a/b", 111, stop, nil, nil)

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
	r.activate("/a", 1, func() error { stoppedA = true; return nil }, nil, nil)

	if err := r.reserve("/b"); err != nil {
		t.Fatal(err)
	}
	r.activate("/b", 2, func() error { stoppedB = true; return nil }, nil, nil)

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

	pid, host, port, stop, _, _, err := starter.Start(ctx, "run")
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

	_, _, _, _, _, _, err := starter.Start(ctx, "run")
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

	_, _, _, _, _, _, err := starter.Start(ctx, "run")
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
	r.activate("/a", 1, func() error { return nil }, exited, nil)
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
	r.activate("/a", 1, func() error { return nil }, exited, nil)

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
	r.activate("/a", 1, func() error { return nil }, oldExited, nil)
	oldEntry, _ := r.get("/a")

	// The old entry is replaced by a new run before the old exit is seen.
	r.remove("/a")
	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 2, func() error { return nil }, nil, nil)

	// Run the old entry's exit watcher to completion, synchronously.
	close(oldExited)
	r.dropOnExit("/a", oldEntry, oldExited)

	entry, ok := r.get("/a")
	if !ok || entry.pid != 2 {
		t.Fatalf("expected the newer entry (pid 2) to remain, got %+v (ok=%v)", entry, ok)
	}
}

// TestRunRegistry_ExitAfterStopAll verifies that the exit watcher is a
// no-op once stopAll has already cleared the registry, including when a
// newer run has since claimed the path.
func TestRunRegistry_ExitAfterStopAll(t *testing.T) {
	r := newRunRegistry()
	exited := make(chan struct{})

	if err := r.reserve("/a"); err != nil {
		t.Fatal(err)
	}
	r.activate("/a", 1, func() error { close(exited); return nil }, exited, nil)
	oldEntry, _ := r.get("/a")

	r.stopAll() // stop closes exited
	if _, ok := r.get("/a"); ok {
		t.Fatal("expected registry to be empty after stopAll")
	}

	if err := r.reserve("/a"); err != nil {
		t.Fatalf("unexpected error reserving after stopAll: %v", err)
	}
	r.activate("/a", 2, func() error { return nil }, nil, nil)

	r.dropOnExit("/a", oldEntry, exited) // exited is already closed
	entry, ok := r.get("/a")
	if !ok || entry.pid != 2 {
		t.Fatalf("expected the newer entry (pid 2) to remain, got %+v (ok=%v)", entry, ok)
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

	_, _, _, stop, exited, _, err := starter.Start(ctx, "run")
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

	_, _, _, stop, exited, _, err := starter.Start(ctx, "run")
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

// TestDefaultProcessStarter_ExitedClosesWithSurvivingChild verifies that
// exited closes when the process dies even though a child it spawned is
// still running and holds the inherited stderr descriptor. Otherwise a
// crashed "func run" whose runner outlives it would leave a stale registry
// entry behind.
func TestDefaultProcessStarter_ExitedClosesWithSurvivingChild(t *testing.T) {
	script := writeTestScript(t, 0)
	body := "#!/bin/sh\necho '{\"host\":\"127.0.0.1\",\"port\":\"9999\"}'\nsleep 10 &\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, stop, exited, _, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("exited not closed after the process died while a child still holds its stderr")
	}
	if err = stop(); err != nil {
		t.Fatalf("unexpected error from stop after self-exit: %v", err)
	}
}

// TestDefaultProcessStarter_ExitsBeforeReadyIncludesStderr verifies that
// the stderr a process wrote before dying is part of the start error.
func TestDefaultProcessStarter_ExitsBeforeReadyIncludesStderr(t *testing.T) {
	script := writeTestScript(t, 0)
	body := "#!/bin/sh\necho 'boom on stderr' >&2\nexit 3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, _, _, _, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected an error when the process exits before becoming ready")
	}
	if !strings.Contains(err.Error(), "boom on stderr") {
		t.Fatalf("expected stderr output in the error, got: %v", err)
	}
}

// TestRunRegistry_ExitedOutputRetained verifies the lifecycle of the output
// kept for a run which exited on its own: it is retrievable, does not block
// a new run, is not kept when the process retains no output, and is cleared
// by a new reserve, removeEntry (run_stop) and stopAll.
func TestRunRegistry_ExitedOutputRetained(t *testing.T) {
	out := func() (string, bool) { return "stdout: x", false }
	stop := func() error { return nil }

	activateAndExit := func(r *runRegistry, path string, output func() (string, bool)) *runEntry {
		t.Helper()
		exited := make(chan struct{})
		if err := r.reserve(path); err != nil {
			t.Fatal(err)
		}
		r.activate(path, 1, stop, nil, output)
		entry, _ := r.get(path)
		close(exited)
		r.dropOnExit(path, entry, exited)
		return entry
	}

	t.Run("retained and not blocking", func(t *testing.T) {
		r := newRunRegistry()
		entry := activateAndExit(r, "/a", out)
		if _, ok := r.get("/a"); ok {
			t.Fatal("exited run must not be active")
		}
		got, ok := r.getExited("/a")
		if !ok || got != entry {
			t.Fatal("expected the exited run to be retained")
		}
		if err := r.reserve("/a"); err != nil {
			t.Fatalf("exited run must not block a new run: %v", err)
		}
		if _, ok := r.getExited("/a"); ok {
			t.Fatal("expected a new reserve to supersede the exited run")
		}
	})

	t.Run("nil output is not retained", func(t *testing.T) {
		r := newRunRegistry()
		activateAndExit(r, "/a", nil)
		if _, ok := r.getExited("/a"); ok {
			t.Fatal("nothing to retain without output")
		}
	})

	t.Run("cleared by removeEntry", func(t *testing.T) {
		r := newRunRegistry()
		entry := activateAndExit(r, "/a", out)
		r.removeEntry("/a", entry)
		if _, ok := r.getExited("/a"); ok {
			t.Fatal("expected removeEntry to discard retained output")
		}
	})

	t.Run("removeEntry of an older entry keeps a newer exited one", func(t *testing.T) {
		r := newRunRegistry()
		old := activateAndExit(r, "/a", out)
		newer := activateAndExit(r, "/a", out)
		r.removeEntry("/a", old)
		if got, ok := r.getExited("/a"); !ok || got != newer {
			t.Fatal("expected the newer exited run to be kept")
		}
	})

	t.Run("cleared by clearExited", func(t *testing.T) {
		r := newRunRegistry()
		activateAndExit(r, "/a", out)
		r.clearExited("/a")
		if _, ok := r.getExited("/a"); ok {
			t.Fatal("expected clearExited to discard retained output")
		}
	})

	t.Run("cleared by stopAll", func(t *testing.T) {
		r := newRunRegistry()
		activateAndExit(r, "/a", out)
		r.stopAll()
		if _, ok := r.getExited("/a"); ok {
			t.Fatal("expected stopAll to discard retained output")
		}
	})
}

// TestTailBuffer_Snapshot verifies that Snapshot returns the retained lines
// and reports dropping only once older lines have actually been discarded.
func TestTailBuffer_Snapshot(t *testing.T) {
	b := newTailBuffer(3)

	if s, dropped := b.Snapshot(); s != "" || dropped {
		t.Fatalf("expected empty, not dropped; got %q, %v", s, dropped)
	}

	b.WriteLine("1")
	b.WriteLine("2")
	b.WriteLine("3")
	if s, dropped := b.Snapshot(); s != "1\n2\n3" || dropped {
		t.Fatalf("expected full buffer, not dropped; got %q, %v", s, dropped)
	}

	b.WriteLine("4")
	if s, dropped := b.Snapshot(); s != "2\n3\n4" || !dropped {
		t.Fatalf("expected oldest line dropped; got %q, %v", s, dropped)
	}
}

// TestDefaultProcessStarter_Output verifies that the output function
// returned by Start exposes stdout and stderr lines with their stream
// prefix, and stays readable after the process has exited.
func TestDefaultProcessStarter_Output(t *testing.T) {
	script := writeTestScript(t, 0)
	body := "#!/bin/sh\n" +
		"echo '{\"host\":\"127.0.0.1\",\"port\":\"9999\"}'\n" +
		"echo 'request handled'\n" +
		"echo 'something broke' >&2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, _, _, stop, exited, output, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected a non-nil output function")
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("expected the script to exit")
	}
	_ = stop()

	waitFor(t, "stdout and stderr lines to be captured", func() bool {
		s, _ := output()
		return strings.Contains(s, "stdout: request handled") && strings.Contains(s, "stderr: something broke")
	})
}
