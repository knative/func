package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

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
	// stop function which gracefully terminates the process (SIGTERM only,
	// no forced kill) and blocks until it has fully exited. stop is
	// idempotent and safe to call even if the process has already exited on
	// its own.
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
	// own signal handler can run its cleanup (job.Stop(), container
	// teardown, etc.), and wait for it to exit on its own. We deliberately
	// do not escalate to SIGKILL: a process that never exits after SIGTERM
	// is a bug in the runner to fix there, not something for the MCP server
	// to paper over by force-killing it (which would skip that cleanup).
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}

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

// runEntry tracks a single "run" invocation. A freshly reserved entry has a
// nil stop (and zero pid) until activate fills it in once the subprocess has
// actually started and become ready; get treats such a pending entry as not
// yet active.
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

// reserve atomically claims path for a new run before the (slow) subprocess
// Start call is made, so that two concurrent "run" calls for the same path
// cannot both spawn a process: the second is rejected here, before anything
// is started. The caller must follow a successful reserve with either
// activate (Start succeeded) or release (Start failed).
func (r *runRegistry) reserve(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byPath[path]; ok {
		if existing.stop == nil {
			return fmt.Errorf("a function is already starting at %q; call run_stop first", path)
		}
		return fmt.Errorf("a function is already running at %q (pid %d); call run_stop first", path, existing.pid)
	}
	r.byPath[path] = &runEntry{}
	return nil
}

// activate fills in the pid/stop for a path previously reserve'd, marking it
// as an active run.
func (r *runRegistry) activate(path string, pid int, stop func() error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPath[path] = &runEntry{pid: pid, stop: stop}
}

// release removes a reservation for path, used when Start fails after a
// successful reserve.
func (r *runRegistry) release(path string) {
	r.remove(path)
}

// get returns the active run entry for path, if any. A path that is
// reserved but not yet activated (still starting) is treated as not active.
func (r *runRegistry) get(path string) (*runEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byPath[path]
	if !ok || e.stop == nil {
		return nil, false
	}
	return e, true
}

func (r *runRegistry) remove(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byPath, path)
}

// stopAll stops every currently active run and clears the registry. It is
// called on normal server shutdown (client disconnect, or SIGINT/SIGTERM
// canceling the server's context) so that no "func run" subprocess is left
// behind holding a port. It does nothing useful on a hard kill of the
// server itself (e.g. SIGKILL), where OS process reaping applies instead.
func (r *runRegistry) stopAll() {
	r.mu.Lock()
	entries := make([]*runEntry, 0, len(r.byPath))
	for _, e := range r.byPath {
		entries = append(entries, e)
	}
	r.byPath = map[string]*runEntry{}
	r.mu.Unlock()

	for _, e := range entries {
		if e.stop != nil {
			_ = e.stop()
		}
	}
}

// resolveRunPath validates the required path input for the run/run_stop
// tools. The MCP server process's working directory is unrelated to the
// caller's, so path must be an absolute path supplied explicitly; there is
// no CWD-based default.
func resolveRunPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be an absolute path, got %q", path)
	}
	return filepath.Clean(path), nil
}
