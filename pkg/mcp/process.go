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

// runReadyTimeout bounds how long "run" waits for the started function to
// report readiness (build + container/host startup can be slow on a cold
// cache). A var (rather than a const) so tests can shrink it.
var runReadyTimeout = 3 * time.Minute

// tailBufferSize is the number of recent output lines retained for
// inclusion in diagnostic error messages and for the run_logs tool.
const tailBufferSize = 200

// maxLineBytes bounds a single retained output line. Together with
// tailBufferSize it bounds the memory held per run: a Function printing one
// huge line must not be able to pin hundreds of megabytes in the registry
// (retained output outlives the process until its path is run again).
const maxLineBytes = 4 * 1024

// lineTruncatedMarker is appended to a retained line which exceeded
// maxLineBytes.
const lineTruncatedMarker = " [line truncated]"

// maxErrorOutputBytes bounds the process output quoted in error messages.
const maxErrorOutputBytes = 16 * 1024

// outputDrainTimeout bounds how long, after the subprocess has exited, we
// wait for the remainder of its stdout and stderr to be captured. It is
// bounded because a child of the subprocess (a runner, a container) may
// outlive its parent and keep the pipes open indefinitely; once it elapses
// the pipes are closed and whatever was captured is what is reported.
const outputDrainTimeout = 500 * time.Millisecond

// startedProcess describes a process returned by processStarter.Start. It is
// an alias for an unnamed struct, rather than a named type, so that the mock
// in pkg/mcp/mock can declare an identical type without importing this
// package; the compiler enforces that the two stay in sync.
type startedProcess = struct {
	// Pid is the process ID.
	Pid int
	// Host and Port are where the process reported itself to be serving.
	Host, Port string
	// Stop gracefully terminates the process (SIGTERM only, no forced
	// kill) and blocks until it has fully exited. It is idempotent and safe
	// to call even if the process has already exited on its own.
	Stop func() error
	// Exited is closed once the process has exited for any reason (stopped
	// via Stop, crashed, or killed externally) and its remaining output has
	// been captured (waiting for that is bounded; see outputDrainTimeout).
	// Callers can thereby learn of a self-exit without calling Stop. A nil
	// channel means the process never reports its exit.
	Exited <-chan struct{}
	// Output snapshots the most recent output (stdout and stderr, each line
	// prefixed with its stream) the process has produced so far, bounded to
	// the last tailBufferSize lines of at most maxLineBytes each. truncated
	// reports whether older lines were dropped to stay within that bound.
	// Output remains callable after the process has exited. A nil Output
	// means the process output is not retained.
	Output func() (lines string, truncated bool)
}

// processStarter starts a long-lived background subcommand (e.g. "func
// run") and waits for it to signal readiness: a line of JSON containing
// non-empty "host" and "port" fields on stdout (produced by passing
// --json). It is abstracted so tests can inject a fake implementation
// instead of spawning real subprocesses.
type processStarter interface {
	// Start begins running subcommand with args in the background. It
	// blocks until the process reports readiness, exits, errors, or ctx is
	// done, whichever happens first. A process which reported readiness and
	// then exited at once is returned successfully, with Exited closed; the
	// caller decides what to make of a run that is already gone.
	//
	// On error, any process that was started is terminated before
	// returning; no process is left running.
	Start(ctx context.Context, subcommand string, args ...string) (startedProcess, error)
}

// defaultProcessStarter starts subprocesses using the server's configured
// command prefix (e.g. "func" or "kn func").
type defaultProcessStarter struct {
	s *Server
}

func (d defaultProcessStarter) Start(ctx context.Context, subcommand string, args ...string) (startedProcess, error) {
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

	// Both streams go through os.Pipes we own, rather than cmd.StdoutPipe
	// or a non-*os.File writer. Either of those makes cmd.Wait responsible
	// for the read side: StdoutPipe is closed by Wait as soon as the process
	// exits (discarding output not yet read, which is exactly the last
	// lines of a crashing process), and a copying goroutine makes Wait block
	// until every holder of the write end (a runner or container which
	// outlives its parent) has closed it, so the exit would never be
	// reported. With *os.File descriptors, Wait returns as soon as the
	// process exits and we drain the pipes ourselves, with a bound.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		cancel()
		return startedProcess{}, fmt.Errorf("unable to create stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		cancel()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return startedProcess{}, fmt.Errorf("unable to create stderr pipe: %w", err)
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err = cmd.Start(); err != nil {
		cancel()
		for _, f := range []*os.File{stdoutR, stdoutW, stderrR, stderrW} {
			_ = f.Close()
		}
		return startedProcess{}, fmt.Errorf("unable to start %q: %w", strings.Join(cmdParts, " "), err)
	}
	// The child holds its own copies; close ours so the readers see EOF
	// once the child (and anything it spawned) is gone.
	_ = stdoutW.Close()
	_ = stderrW.Close()

	tail := newTailBuffer(tailBufferSize)
	ready := make(chan readyResult, 1)

	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		defer stdoutR.Close()
		scanForReady(stdoutR, tail, ready)
	}()
	go func() {
		defer readers.Done()
		defer stderrR.Close()
		readLines(stderrR, func(line string) { tail.WriteLine("stderr: " + line) })
	}()
	readersDone := make(chan struct{})
	go func() {
		readers.Wait()
		close(readersDone)
	}()

	done := make(chan struct{})   // the process has exited
	exited := make(chan struct{}) // ... and its output has been captured
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
		select {
		case <-readersDone:
		case <-time.After(outputDrainTimeout):
			// Something outlived the process and holds the pipes open.
			// Closing the read ends unblocks the readers.
			_ = stdoutR.Close()
			_ = stderrR.Close()
		}
		close(exited)
	}()

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			select {
			case <-done:
				return // already exited; nothing to signal
			default:
			}
			cancel()
			<-done
		})
		return nil
	}

	started := func(r readyResult) startedProcess {
		return startedProcess{
			Pid:    cmd.Process.Pid,
			Host:   r.host,
			Port:   r.port,
			Stop:   stop,
			Exited: exited,
			Output: tail.Snapshot,
		}
	}

	select {
	case r := <-ready:
		return started(r), nil
	case <-done:
		_ = stop()
		// Both waits are bounded (see outputDrainTimeout, which closes the
		// read ends and thereby unblocks the readers). They let the failure
		// message include the final output, and settle whether a readiness
		// line was read.
		<-exited
		<-readersDone
		// A process can print its readiness line and then exit immediately,
		// in which case the stdout reader may deliver that line only after
		// cmd.Wait has returned. Such a process did become ready, so report
		// it as started rather than discarding the line: the caller sees it
		// is already gone via Exited, which is closed.
		select {
		case r := <-ready:
			return started(r), nil
		default:
		}
		out, _ := truncateTail(tail.String(), maxErrorOutputBytes)
		return startedProcess{}, fmt.Errorf("process exited before becoming ready (%v)\noutput:\n%s", waitErr, out)
	case <-ctx.Done():
		_ = stop()
		<-exited
		out, _ := truncateTail(tail.String(), maxErrorOutputBytes)
		return startedProcess{}, fmt.Errorf("timed out waiting for function to become ready\noutput so far:\n%s", out)
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
// stdout, generally because it exited) or is closed.
func scanForReady(r io.Reader, tail *tailBuffer, ready chan<- readyResult) {
	sent := false
	readLines(r, func(line string) {
		tail.WriteLine("stdout: " + line)
		if sent {
			return
		}
		if host, port, ok := parseReadyLine(line); ok {
			sent = true
			ready <- readyResult{host: host, port: port}
		}
	})
}

// readLines calls fn with each line read from r until r is exhausted,
// returns an error or is closed. Unlike bufio.Scanner it never gives up on
// an over-long line (which would stop the drain and block the child on a
// full pipe): a line longer than maxLineBytes is cut and marked, and the
// rest of it discarded. A final line without a terminating newline is
// delivered too, as that is how a dying process often ends its output.
// Lines are valid UTF-8; invalid bytes are dropped.
func readLines(r io.Reader, fn func(line string)) {
	br := bufio.NewReaderSize(r, 4*1024)
	var line []byte
	var over bool
	emit := func() {
		s := strings.ToValidUTF8(string(line), "")
		if over {
			s += lineTruncatedMarker
		}
		line, over = line[:0], false
		fn(s)
	}
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if len(line) > 0 || over {
				emit()
			}
			return
		}
		if room := maxLineBytes - len(line); len(chunk) > room {
			chunk, over = chunk[:room], true
		}
		line = append(line, chunk...)
		if !isPrefix {
			emit()
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
// lines from a subprocess, kept for inclusion in error messages and for the
// run_logs tool.
type tailBuffer struct {
	mu      sync.Mutex
	lines   []string
	max     int
	dropped bool // true once any older line has been discarded
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
		t.dropped = true
	}
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}

// Snapshot returns the retained lines joined by newlines, and whether older
// lines have been discarded to stay within the buffer's bound.
func (t *tailBuffer) Snapshot() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n"), t.dropped
}

// runEntry tracks a single "run" invocation. A freshly reserved entry has a
// nil stop (and zero pid) until activate fills it in once the subprocess has
// actually started and become ready; get treats such a pending entry as not
// yet active.
type runEntry struct {
	pid  int
	stop func() error
	// exited is closed once the process has exited; see
	// startedProcess.Exited. May be nil.
	exited <-chan struct{}
	// output snapshots the process's retained output; see
	// startedProcess.Output. May be nil.
	output func() (lines string, truncated bool)
}

// hasExited reports whether the process is known to have exited.
func (e *runEntry) hasExited() bool {
	select {
	case <-e.exited: // a nil channel never fires
		return true
	default:
		return false
	}
}

// runRegistry tracks local function runs, keyed by the resolved absolute path
// of the Function being run. It is the single source of truth used by the
// "run", "run_stop" and "run_logs" tools.
type runRegistry struct {
	mu     sync.Mutex
	byPath map[string]*runEntry
	// finished holds, per path, the last run which is no longer active,
	// whether its process exited on its own or was stopped. It does not
	// block a new run, but is kept so that run_logs can still show what the
	// Function printed, which matters most after a crash. Only runs whose
	// output is retained are kept. It is replaced when a new run claims the
	// path and cleared on stopAll.
	finished map[string]*runEntry
}

func newRunRegistry() *runRegistry {
	return &runRegistry{byPath: map[string]*runEntry{}, finished: map[string]*runEntry{}}
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
	delete(r.finished, path) // a new run supersedes the previous one's output
	return nil
}

// activate fills in the pid/stop/exited/output for a path previously
// reserve'd, marking it as an active run, and returns the entry. exited and
// output may be nil.
//
// If exited is non-nil, the entry is retired as soon as exited is closed, so
// that a process which dies on its own (crash, killed externally) does not
// leave a stale entry behind blocking later runs for the same path. Only this
// exact entry is retired: if it has already been removed and the path
// re-reserved by a newer run, that newer run is left untouched.
func (r *runRegistry) activate(path string, pid int, stop func() error, exited <-chan struct{}, output func() (string, bool)) *runEntry {
	entry := &runEntry{pid: pid, stop: stop, exited: exited, output: output}
	r.mu.Lock()
	r.byPath[path] = entry
	r.mu.Unlock()

	if exited != nil {
		go r.dropOnExit(path, entry, exited)
	}
	return entry
}

// dropOnExit blocks until exited is closed and then retires entry. It is run
// in its own goroutine by activate; it is a separate method so tests can
// call it synchronously.
func (r *runRegistry) dropOnExit(path string, entry *runEntry, exited <-chan struct{}) {
	<-exited
	r.retire(path, entry)
}

// retire makes path inactive (so it can be run again) if entry is still the
// active entry for it, retaining entry so that its final output stays
// readable via lookup. It is used both when a process exits on its own and by
// run_stop once it has stopped one. Retiring an entry which is no longer the
// active one (already retired, or superseded by a newer run) does nothing.
func (r *runRegistry) retire(path string, entry *runEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byPath[path] != entry {
		return
	}
	delete(r.byPath, path)
	if entry.output != nil {
		r.finished[path] = entry
	}
}

// getFinished returns the last run at path which is no longer active, if its
// output was retained.
func (r *runRegistry) getFinished(path string) (*runEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.finished[path]
	return e, ok
}

// lookup returns the run to read output from for path: the active run if
// there is one, otherwise the last finished one. running reports whether the
// process is still alive. Both are decided under a single lock, and a process
// known to have exited is not reported as running merely because its exit
// has not been processed yet.
func (r *runRegistry) lookup(path string) (entry *runEntry, running, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, found := r.byPath[path]; found && e.stop != nil {
		return e, !e.hasExited(), true
	}
	e, found := r.finished[path]
	return e, false, found
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

// stopAll stops every currently active run and clears the registry,
// including retained output. It is called on normal server shutdown (client
// disconnect, or SIGINT/SIGTERM canceling the server's context) so that no
// "func run" subprocess is left behind holding a port. It does nothing
// useful on a hard kill of the server itself (e.g. SIGKILL), where OS process
// reaping applies instead.
//
// Exit watchers started by activate outlive this call and fire once each
// stopped process exits. They are harmless: a watcher only ever retires the
// exact entry it was created for, and every entry has been dropped from the
// map here.
func (r *runRegistry) stopAll() {
	r.mu.Lock()
	entries := make([]*runEntry, 0, len(r.byPath))
	for _, e := range r.byPath {
		entries = append(entries, e)
	}
	r.byPath = map[string]*runEntry{}
	r.finished = map[string]*runEntry{}
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
