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

	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proc.Pid <= 0 {
		t.Fatalf("expected positive pid, got %d", proc.Pid)
	}
	if proc.Host != "127.0.0.1" || proc.Port != "9999" {
		t.Fatalf("expected 127.0.0.1:9999, got %s:%s", proc.Host, proc.Port)
	}

	if err = proc.Stop(); err != nil {
		t.Fatalf("unexpected error from stop: %v", err)
	}
	// stop must be idempotent.
	if err = proc.Stop(); err != nil {
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

	_, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected an error when the process exits before becoming ready")
	}
}

// TestDefaultProcessStarter_ReadyThenImmediateExit verifies that a process
// which emits its readiness line and dies at once is still reported as
// started, and that its exit is signalled. The readiness line is read by a
// goroutine which can deliver it only after the process has been reaped,
// and losing that race must not be mistaken for never becoming ready.
func TestDefaultProcessStarter_ReadyThenImmediateExit(t *testing.T) {
	// Repeated: the failure mode is a race.
	for i := 0; i < 20; i++ {
		func() {
			starter := newScriptStarter(t, readyEcho+"exit 1\n")

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			proc, err := starter.Start(ctx, "run")
			if err != nil {
				t.Fatalf("iteration %d: unexpected error: %v", i, err)
			}
			if proc.Host != "127.0.0.1" || proc.Port != "9999" {
				t.Fatalf("iteration %d: got host %q port %q", i, proc.Host, proc.Port)
			}
			select {
			case <-proc.Exited:
			case <-time.After(10 * time.Second):
				t.Fatalf("iteration %d: expected the exit to be reported", i)
			}
		}()
	}
}

// TestDefaultProcessStarter_Timeout verifies that Start gives up and returns
// an error if the process never becomes ready within the given context.
func TestDefaultProcessStarter_Timeout(t *testing.T) {
	script := writeTestScript(t, 3*time.Second, `{"host":"127.0.0.1","port":"9999"}`)
	starter := newTestStarter(t, script)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	_, err := starter.Start(ctx, "run")
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

	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-proc.Exited:
	case <-time.After(5 * time.Second):
		t.Fatal("expected exited to be closed after the process exited on its own")
	}
	if err = proc.Stop(); err != nil {
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

	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-proc.Exited:
		t.Fatal("exited must not be closed while the process is running")
	default:
	}

	if err = proc.Stop(); err != nil {
		t.Fatalf("unexpected error from stop: %v", err)
	}
	// Exited follows shortly after the process: it also waits for the output
	// to be captured.
	select {
	case <-proc.Exited:
	case <-time.After(5 * time.Second):
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

	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-proc.Exited:
	case <-time.After(3 * time.Second):
		t.Fatal("exited not closed after the process died while a child still holds its stderr")
	}
	if err = proc.Stop(); err != nil {
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

	_, err := starter.Start(ctx, "run")
	if err == nil {
		t.Fatal("expected an error when the process exits before becoming ready")
	}
	if !strings.Contains(err.Error(), "boom on stderr") {
		t.Fatalf("expected stderr output in the error, got: %v", err)
	}
}

// TestRunRegistry_FinishedOutputRetained verifies the lifecycle of the
// output kept for a run which is no longer active: it is retrievable, does
// not block a new run, is not kept when the process retains no output, and is
// replaced by a new reserve and cleared by stopAll.
func TestRunRegistry_FinishedOutputRetained(t *testing.T) {
	out := func() (string, bool) { return "stdout: x", false }
	stop := func() error { return nil }

	activateAndRetire := func(r *runRegistry, path string, output func() (string, bool)) *runEntry {
		t.Helper()
		if err := r.reserve(path); err != nil {
			t.Fatal(err)
		}
		entry := r.activate(path, 1, stop, nil, output)
		r.retire(path, entry)
		return entry
	}

	t.Run("retained and not blocking", func(t *testing.T) {
		r := newRunRegistry()
		entry := activateAndRetire(r, "/a", out)
		if _, ok := r.get("/a"); ok {
			t.Fatal("a retired run must not be active")
		}
		got, ok := r.getFinished("/a")
		if !ok || got != entry {
			t.Fatal("expected the retired run to be retained")
		}
		if err := r.reserve("/a"); err != nil {
			t.Fatalf("a retired run must not block a new run: %v", err)
		}
		if _, ok := r.getFinished("/a"); ok {
			t.Fatal("expected a new reserve to supersede the retired run")
		}
	})

	t.Run("nil output is not retained", func(t *testing.T) {
		r := newRunRegistry()
		activateAndRetire(r, "/a", nil)
		if _, ok := r.getFinished("/a"); ok {
			t.Fatal("nothing to retain without output")
		}
	})

	t.Run("retiring a superseded entry does nothing", func(t *testing.T) {
		r := newRunRegistry()
		old := activateAndRetire(r, "/a", out)
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		newer := r.activate("/a", 2, stop, nil, out)

		r.retire("/a", old) // late retirement of the older run
		if got, ok := r.get("/a"); !ok || got != newer {
			t.Fatal("expected the newer active run to be untouched")
		}
	})

	t.Run("retiring twice is harmless", func(t *testing.T) {
		r := newRunRegistry()
		entry := activateAndRetire(r, "/a", out)
		r.retire("/a", entry)
		if got, ok := r.getFinished("/a"); !ok || got != entry {
			t.Fatal("expected the entry to remain retained")
		}
	})

	t.Run("cleared by stopAll", func(t *testing.T) {
		r := newRunRegistry()
		activateAndRetire(r, "/a", out)
		r.stopAll()
		if _, ok := r.getFinished("/a"); ok {
			t.Fatal("expected stopAll to discard retained output")
		}
	})

	t.Run("exit watcher retains output", func(t *testing.T) {
		r := newRunRegistry()
		exited := make(chan struct{})
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		r.activate("/a", 1, stop, exited, out)
		close(exited)
		waitFor(t, "output to be retained after exit", func() bool {
			_, ok := r.getFinished("/a")
			return ok
		})
	})
}

// TestRunRegistry_Lookup verifies which run lookup picks and whether it is
// reported as running, including the window where the process has exited but
// the exit has not been processed yet.
func TestRunRegistry_Lookup(t *testing.T) {
	out := func() (string, bool) { return "", false }
	stop := func() error { return nil }

	t.Run("none", func(t *testing.T) {
		r := newRunRegistry()
		if _, _, ok := r.lookup("/a"); ok {
			t.Fatal("expected no run")
		}
	})

	t.Run("pending reservation is not a run", func(t *testing.T) {
		r := newRunRegistry()
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := r.lookup("/a"); ok {
			t.Fatal("expected a pending reservation not to be found")
		}
	})

	t.Run("active", func(t *testing.T) {
		r := newRunRegistry()
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		r.activate("/a", 1, stop, make(chan struct{}), out)
		if _, running, ok := r.lookup("/a"); !ok || !running {
			t.Fatalf("expected a running run, got running=%v ok=%v", running, ok)
		}
	})

	t.Run("exited but not yet retired", func(t *testing.T) {
		r := newRunRegistry()
		exited := make(chan struct{})
		close(exited)
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		// Bypass activate so no watcher retires the entry: this is the
		// window between the process exiting and the watcher running.
		r.byPath["/a"] = &runEntry{pid: 1, stop: stop, exited: exited, output: out}
		if _, running, ok := r.lookup("/a"); !ok || running {
			t.Fatalf("expected an exited run not reported as running, got running=%v ok=%v", running, ok)
		}
	})

	t.Run("finished", func(t *testing.T) {
		r := newRunRegistry()
		if err := r.reserve("/a"); err != nil {
			t.Fatal(err)
		}
		r.retire("/a", r.activate("/a", 1, stop, nil, out))
		if _, running, ok := r.lookup("/a"); !ok || running {
			t.Fatalf("expected a finished run, got running=%v ok=%v", running, ok)
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

	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proc.Output == nil {
		t.Fatal("expected a non-nil output function")
	}
	select {
	case <-proc.Exited:
	case <-time.After(5 * time.Second):
		t.Fatal("expected the script to exit")
	}
	_ = proc.Stop()

	// Exited is closed only once the output has been captured, so no
	// polling is needed.
	got, _ := proc.Output()
	for _, want := range []string{"stdout: request handled", "stderr: something broke"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

// newScriptStarter writes body as an executable script and returns a starter
// for it.
func newScriptStarter(t *testing.T, body string) defaultProcessStarter {
	t.Helper()
	script := writeTestScript(t, 0)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return newTestStarter(t, script)
}

const readyEcho = "echo '{\"host\":\"127.0.0.1\",\"port\":\"9999\"}'\n"

// startAndWaitExit starts the script, waits for it to exit and returns its
// captured output.
func startAndWaitExit(t *testing.T, starter defaultProcessStarter) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	proc, err := starter.Start(ctx, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-proc.Exited:
	case <-time.After(10 * time.Second):
		t.Fatal("expected the script to exit")
	}
	return proc.Output()
}

// TestDefaultProcessStarter_FinalStdoutNotLost verifies that the last lines
// a process writes to stdout before dying are captured. They are the lines
// which explain a crash, and are lost if the stdout pipe is closed on exit
// before it has been drained.
func TestDefaultProcessStarter_FinalStdoutNotLost(t *testing.T) {
	var b strings.Builder
	b.WriteString(readyEcho)
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&b, "echo 'line %d'\n", i)
	}
	b.WriteString("echo 'fatal: the last words'\nexit 1\n")

	// Repeated: the failure mode is a race.
	for i := 0; i < 20; i++ {
		got, _ := startAndWaitExit(t, newScriptStarter(t, b.String()))
		if !strings.Contains(got, "stdout: line 149\nstdout: fatal: the last words") {
			t.Fatalf("iteration %d: final stdout lines lost; output ends with %q", i, got[max(0, len(got)-120):])
		}
	}
}

// TestDefaultProcessStarter_PartialLinesJoined verifies that a line written
// in several pieces is one line of output, not one per write.
func TestDefaultProcessStarter_PartialLinesJoined(t *testing.T) {
	starter := newScriptStarter(t, readyEcho+
		"printf 'abc' >&2\nsleep 0.2\nprintf 'def\\n' >&2\n"+
		"printf 'uvw'\nsleep 0.2\nprintf 'xyz\\n'\n")

	got, _ := startAndWaitExit(t, starter)
	for _, want := range []string{"stderr: abcdef", "stdout: uvwxyz"} {
		if !strings.Contains(got, want+"\n") && !strings.HasSuffix(got, want) {
			t.Fatalf("expected %q as a single line, got %q", want, got)
		}
	}
	if strings.Contains(got, "stderr: abc\n") || strings.Contains(got, "stderr: def") {
		t.Fatalf("a partially written line was split: %q", got)
	}
}

// TestDefaultProcessStarter_UnterminatedFinalLine verifies that a last line
// with no trailing newline is still captured.
func TestDefaultProcessStarter_UnterminatedFinalLine(t *testing.T) {
	starter := newScriptStarter(t, readyEcho+"printf 'out without newline'\nprintf 'err without newline' >&2\n")

	got, _ := startAndWaitExit(t, starter)
	for _, want := range []string{"stdout: out without newline", "stderr: err without newline"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got %q", want, got)
		}
	}
}

// TestDefaultProcessStarter_LongLineBounded verifies that a line longer than
// maxLineBytes is cut and marked rather than retained whole, and that it does
// not stop later output from being drained (which would block the child).
func TestDefaultProcessStarter_LongLineBounded(t *testing.T) {
	starter := newScriptStarter(t, readyEcho+
		"head -c 2000000 /dev/zero | tr '\\0' x\necho\n"+
		"head -c 2000000 /dev/zero | tr '\\0' y >&2\necho >&2\n"+
		"echo 'after the long lines'\n")

	got, _ := startAndWaitExit(t, starter)
	if !strings.Contains(got, "stdout: after the long lines") {
		t.Fatalf("output after a very long line was lost: %.200q", got)
	}
	if strings.Count(got, lineTruncatedMarker) != 2 {
		t.Fatalf("expected both long lines to be marked as truncated, got %d markers", strings.Count(got, lineTruncatedMarker))
	}
	if limit := 4 * (maxLineBytes + len("stdout: ") + len(lineTruncatedMarker)); len(got) > limit {
		t.Fatalf("expected retained output bounded to ~%d bytes, got %d", limit, len(got))
	}
}

// TestReadLines covers line splitting in isolation.
func TestReadLines(t *testing.T) {
	long := strings.Repeat("a", maxLineBytes+10)
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"lines", "a\nb\n", []string{"a", "b"}},
		{"no trailing newline", "a\nb", []string{"a", "b"}},
		{"crlf", "a\r\nb\r\n", []string{"a", "b"}},
		{"blank lines kept", "a\n\nb\n", []string{"a", "", "b"}},
		{"long line cut and marked", long + "\nz\n", []string{long[:maxLineBytes] + lineTruncatedMarker, "z"}},
		{"line of exactly the limit", strings.Repeat("a", maxLineBytes) + "\n", []string{strings.Repeat("a", maxLineBytes)}},
		{"invalid utf-8 dropped", "a\xffb\n", []string{"ab"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			readLines(strings.NewReader(tc.in), func(l string) { got = append(got, l) })
			if len(got) != len(tc.want) {
				t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("line %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestDefaultProcessStarter_SurvivingChildDoesNotLeakReaders verifies that
// when a child outlives the process and keeps its pipes open, the output
// readers are still released once the drain timeout elapses, so no goroutine
// or descriptor stays pinned for the child's lifetime.
func TestDefaultProcessStarter_SurvivingChildDoesNotLeakReaders(t *testing.T) {
	starter := newScriptStarter(t, readyEcho+"echo 'parent output'\nsleep 20 &\nexit 1\n")

	before := runtime.NumGoroutine()
	got, _ := startAndWaitExit(t, starter)
	if !strings.Contains(got, "stdout: parent output") {
		t.Fatalf("expected the parent's output, got %q", got)
	}
	waitFor(t, "reader goroutines to be released", func() bool {
		return runtime.NumGoroutine() <= before
	})
}
