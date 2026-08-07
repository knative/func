package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// runStopGrace is the delay after sending SIGTERM before a subprocess that
// has not yet exited is force-killed with SIGKILL. A var (rather than a
// const) so tests can shrink it.
var runStopGrace = 10 * time.Second

// runReadyTimeout bounds how long "run" waits for the started function to
// report readiness (build + container/host startup can be slow on a cold
// cache). A var (rather than a const) so tests can shrink it.
var runReadyTimeout = 3 * time.Minute

// tailBufferSize is the number of recent output lines retained for
// inclusion in diagnostic error messages.
const tailBufferSize = 200

// processStarter starts a long-lived background subcommand (e.g. "func
// run") and waits for it to signal readiness: a line of JSON containing
// non-empty "host" and "port" fields on stdout (produced by passing
// --json). It is abstracted so tests can inject a fake implementation
// instead of spawning real subprocesses.
type processStarter interface {
	// Start begins running subcommand with args in the background. It
	// blocks until the process reports readiness, exits, errors, or ctx is
	// done, whichever happens first.
	//
	// On success it returns the process ID, host, and port, along with a
	// stop function which gracefully terminates the process (SIGTERM, then
	// SIGKILL after a grace period) and blocks until it has fully exited.
	// stop is idempotent and safe to call even if the process has already
	// exited on its own.
	//
	// On error, any process that was started is terminated before
	// returning; no process is left running.
	Start(ctx context.Context, subcommand string, args ...string) (pid int, host, port string, stop func() error, err error)
}

// defaultProcessStarter starts subprocesses using the server's configured
// command prefix (e.g. "func" or "kn func").
type defaultProcessStarter struct {
	s *Server
}

func (d defaultProcessStarter) Start(ctx context.Context, subcommand string, args ...string) (pid int, host, port string, stop func() error, err error) {
	cmdParts := buildArgs(d.s.prefix, subcommand, args)

	// The subprocess must outlive this call (and the request context that
	// bounds it), so it is given its own independent, cancelable context.
	// The incoming ctx is used only to bound how long we wait below for
	// readiness.
	procCtx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, cmdParts[0], cmdParts[1:]...)
	// Send SIGTERM (not the default SIGKILL) on cancellation so the child's
	// own signal handler can run its cleanup (job.Stop()). WaitDelay forces
	// a SIGKILL if it has not exited within the grace period.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = runStopGrace

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return 0, "", "", nil, fmt.Errorf("unable to create stdout pipe: %w", err)
	}
	tail := newTailBuffer(tailBufferSize)
	cmd.Stderr = tail

	if err = cmd.Start(); err != nil {
		cancel()
		return 0, "", "", nil, fmt.Errorf("unable to start %q: %w", strings.Join(cmdParts, " "), err)
	}

	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()

	ready := make(chan readyResult, 1)
	go scanForReady(stdout, tail, ready)

	var stopOnce sync.Once
	stop = func() error {
		stopOnce.Do(func() {
			select {
			case <-exited:
				return // already exited; nothing to signal
			default:
			}
			cancel()
			<-exited
		})
		return nil
	}

	select {
	case r := <-ready:
		return cmd.Process.Pid, r.host, r.port, stop, nil
	case <-exited:
		_ = stop()
		return 0, "", "", nil, fmt.Errorf("process exited before becoming ready (%v)\noutput:\n%s", waitErr, tail.String())
	case <-ctx.Done():
		_ = stop()
		return 0, "", "", nil, fmt.Errorf("timed out waiting for function to become ready\noutput so far:\n%s", tail.String())
	}
}

// readyResult carries the host/port parsed from a subprocess's --json
// readiness line.
type readyResult struct {
	host, port string
}

// runReadyLine is the shape of the single line of JSON that "func run
// --json" prints once the function is up and healthy.
type runReadyLine struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

// scanForReady drains r line-by-line for the lifetime of the process
// (required to avoid the child blocking on a full stdout pipe once it
// starts streaming logs), sending exactly once on ready as soon as a valid
// readiness line is found. Every line is also recorded in tail for
// diagnostics. It returns once r reaches EOF (i.e. the process closed
// stdout, generally because it exited).
func scanForReady(r io.ReadCloser, tail *tailBuffer, ready chan<- readyResult) {
	defer r.Close()
	sent := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		tail.WriteLine("stdout: " + line)
		if !sent {
			if host, port, ok := parseReadyLine(line); ok {
				sent = true
				ready <- readyResult{host: host, port: port}
			}
		}
	}
}

// parseReadyLine attempts to parse line as the --json readiness output.
func parseReadyLine(line string) (host, port string, ok bool) {
	var out runReadyLine
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return "", "", false
	}
	if out.Host == "" || out.Port == "" {
		return "", "", false
	}
	return out.Host, out.Port, true
}

// tailBuffer is a bounded, concurrency-safe buffer of the most recent output
// lines from a subprocess, kept for inclusion in error messages. It
// implements io.Writer so it can be used directly as a Cmd's Stderr.
type tailBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (t *tailBuffer) WriteLine(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, line)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		t.WriteLine("stderr: " + line)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}

// runEntry tracks a single active "run" invocation.
type runEntry struct {
	pid  int
	stop func() error
}

// runRegistry tracks active local function runs, keyed by the resolved
// absolute path of the Function being run. It is the single source of
// truth used by both the "run" and "run_stop" tools.
type runRegistry struct {
	mu     sync.Mutex
	byPath map[string]*runEntry
}

func newRunRegistry() *runRegistry {
	return &runRegistry{byPath: map[string]*runEntry{}}
}

// add registers a new run for path. It returns an error if a run is already
// registered for that path, in which case the caller is responsible for
// stopping the process it just started (it has not been registered).
func (r *runRegistry) add(path string, pid int, stop func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byPath[path]; ok {
		return fmt.Errorf("a function is already running at %q (pid %d); call run_stop first", path, existing.pid)
	}
	r.byPath[path] = &runEntry{pid: pid, stop: stop}
	return nil
}

func (r *runRegistry) get(path string) (*runEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byPath[path]
	return e, ok
}

func (r *runRegistry) remove(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byPath, path)
}

// resolveRunPath resolves the optional path input for the run/run_stop
// tools to an absolute path, defaulting to the server process's current
// working directory when omitted. Both tools must resolve paths the same
// way so that a "run" followed by a "run_stop" with equivalent path inputs
// (e.g. relative vs. absolute) refer to the same registry entry.
func resolveRunPath(path *string) (string, error) {
	if path == nil || *path == "" {
		return os.Getwd()
	}
	return filepath.Abs(*path)
}
