package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"knative.dev/func/pkg/kafkaruntime"
)

// Local Kafka delivery
//
// When run.kafka is configured, `func run` starts the Kafka runtime (the same
// program injected as the sidecar on the cluster) as a child process next to
// the function. The runtime consumes records and POSTs each one as a CloudEvent
// to the function's local address, committing the offset only on a 2xx, so a
// function behaves the same locally as deployed and needs no Kafka code in any
// language. It is started from Client.Run, after the runner reports the
// function ready, so it serves every runner alike: the host runner's child
// process and the container runner's published port.

const (
	// kafkaRuntimeStopTimeout bounds how long a signaled Kafka runtime may take
	// to finish an in-flight delivery and leave its consumer group (committing
	// final offsets) before it is killed.
	kafkaRuntimeStopTimeout = 10 * time.Second

	// kafkaRuntimeReadyInterval is how often the runtime's readiness is polled.
	kafkaRuntimeReadyInterval = 500 * time.Millisecond

	// kafkaRuntimeLogPrefix marks the runtime's output in `func run` logs.
	kafkaRuntimeLogPrefix = "[kafka] "

	// kafkaRuntimeMaxLine caps a buffered, not yet newline-terminated line of
	// runtime output before it is written out regardless.
	kafkaRuntimeMaxLine = 64 * 1024
)

// kafkaRuntimeBinary returns the Kafka runtime executable. A variable so tests
// can substitute a fake runtime.
var kafkaRuntimeBinary = kafkaruntime.Binary

// kafkaConfigured reports whether run.kafka is complete enough to consume. It
// is the same condition under which the cluster deployers inject the sidecar.
func kafkaConfigured(k *KafkaConfig) bool {
	return k != nil && k.Brokers != "" && k.Topic != "" && k.ConsumerGroup != ""
}

// startKafkaRuntime starts the Kafka runtime for a function whose run.kafka is
// configured, delivering to the function running as job, and waits up to
// timeout for it to become ready. The runtime is ready only once its consumer
// has joined the group and been assigned partitions, so an unreachable broker,
// a missing topic or bad credentials fail here rather than leaving a run that
// silently receives nothing.
//
// Once started, the runtime is stopped with the job (before the function, so it
// stops consuming first), and an unexpected exit is reported on job.Errors like
// the function's own. When run.kafka is not configured this does nothing; when
// no runtime is available for this platform it warns and does nothing.
func startKafkaRuntime(ctx context.Context, job *Job, timeout time.Duration) error {
	k := job.Function.Run.Kafka
	if !kafkaConfigured(k) {
		return nil
	}

	// Validate as the cluster deployers do before building the sidecar, so a
	// malformed value fails with func's message rather than as a runtime crash.
	if errs := ValidateKafkaDelivery(k); len(errs) > 0 {
		return fmt.Errorf("invalid run.kafka.delivery: %s", strings.Join(errs, "; "))
	}
	config, warnings, err := kafkaRuntimeEnv(k)
	if err != nil {
		return err
	}

	bin, err := kafkaRuntimeBinary()
	if errors.Is(err, kafkaruntime.ErrUnavailable) {
		fmt.Fprintf(os.Stderr, "Warning: run.kafka is configured, but this func build includes no Kafka runtime for %s/%s, so the function will not receive records from topic %q. To deliver them locally, set %s to the path of an fkafka-runtime %s binary.\n",
			runtime.GOOS, runtime.GOARCH, k.Topic, kafkaruntime.BinaryEnv, kafkaruntime.Version)
		return nil
	} else if err != nil {
		return fmt.Errorf("preparing the Kafka runtime: %w", err)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// The runtime serves its health endpoints on a free loopback port of its
	// own, so it never collides with the function's.
	port, err := freeLoopbackPort()
	if err != nil {
		return fmt.Errorf("choosing a port for the Kafka runtime: %w", err)
	}
	listen := net.JoinHostPort(defaultRunHost, port)
	target := kafkaFunctionTarget(job.Host, job.Port)

	env := append(withoutKafkaRuntimeEnv(os.Environ()),
		"FUNCTION_TARGET="+target,
		"LISTEN_ADDRESS="+listen)
	env = append(env, config...)

	if job.verbose {
		fmt.Printf("Starting Kafka runtime %v: topic %q, group %q, brokers %v, delivering to %v (health %v)\n",
			bin, k.Topic, k.ConsumerGroup, k.Brokers, target, listen)
	}
	rt, err := startKafkaRuntimeProcess(ctx, bin, env, funcJobsDir(job.Function))
	if err != nil {
		return err
	}
	if err = rt.waitReady(ctx, "http://"+listen+readinessEndpoint, timeout, job.verbose); err != nil {
		rt.stop()
		return err
	}

	// Stop the runtime with the job, ahead of the function.
	stopFunction := job.onStop
	job.onStop = func() error {
		rt.stop()
		if stopFunction == nil {
			return nil
		}
		return stopFunction()
	}
	go rt.reportExit(ctx, job.Errors)
	return nil
}

// kafkaRuntimeEnv returns the runtime's configuration environment for
// run.kafka: the variables KafkaConnectionEnvs and KafkaDeliveryEnvs give, the
// same the cluster deployers set on the sidecar, with SASL credentials
// resolved for a local run (see resolveKafkaCredential). TLS files are read by
// the runtime, which runs on this host, so their paths are host paths whichever
// runner runs the function. The returned warnings are to be shown to the user.
func kafkaRuntimeEnv(k *KafkaConfig) (env, warnings []string, err error) {
	for _, e := range append(KafkaConnectionEnvs(k), KafkaDeliveryEnvs(k)...) {
		value := e.Value
		if e.Field != "" {
			var warning string
			if value, warning, err = resolveKafkaCredential(e.Field, e.Value); err != nil {
				return nil, nil, err
			}
			if warning != "" {
				warnings = append(warnings, warning)
			}
		}
		env = append(env, e.Name+"="+value)
	}
	return env, warnings, nil
}

// resolveKafkaCredential resolves a run.kafka.sasl value for a local run the
// way Interpolate resolves every run.envs value: a {{ env:NAME }} reference is
// read from the local environment (an error if unset) and an inline value is
// used as is. A cluster {{ secret:... }} or {{ configMap:... }} reference has
// no local meaning and passes through unresolved, as it does for run.envs; as
// the runtime would then authenticate with the reference text itself, a
// warning naming the field is returned.
func resolveKafkaCredential(field, value string) (resolved, warning string, err error) {
	name := field
	envs, err := Interpolate([]Env{{Name: &name, Value: &value}})
	if err != nil {
		return "", "", fmt.Errorf("resolving %s: %w", field, err)
	}
	resolved = envs[name]
	if resolved == value && strings.HasPrefix(value, "{{") {
		warning = fmt.Sprintf("%s is the cluster reference %s, which func run cannot resolve locally; the Kafka runtime will receive it verbatim and authentication will fail", field, value)
	}
	return resolved, warning, nil
}

// withoutKafkaRuntimeEnv returns environ without the variables that configure
// the Kafka runtime, so it is configured by run.kafka alone, as the sidecar is,
// and not by whatever happens to be set in the user's shell.
func withoutKafkaRuntimeEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		name, _, _ := strings.Cut(e, "=")
		name = strings.ToUpper(name) // names are case-insensitive on Windows
		if strings.HasPrefix(name, "KAFKA_") ||
			strings.HasPrefix(name, "FUNCTION_DELIVERY_") ||
			name == "FUNCTION_TARGET" ||
			name == "FUNCTION_REQUEST_TIMEOUT" ||
			name == "LISTEN_ADDRESS" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// kafkaFunctionTarget returns the URL the runtime delivers to: the function's
// address as reported by the runner, with an unspecified (all-interfaces)
// host replaced by loopback, which is where the runtime reaches it.
func kafkaFunctionTarget(host, port string) string {
	switch ip := net.ParseIP(host); {
	case host == "":
		host = defaultRunHost
	case ip != nil && ip.IsUnspecified() && ip.To4() != nil:
		host = defaultRunHost
	case ip != nil && ip.IsUnspecified():
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// freeLoopbackPort returns a port the OS reports free on the loopback
// interface. As with choosePort, another process could take it before the
// runtime binds it.
func freeLoopbackPort() (string, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(defaultRunHost, "0"))
	if err != nil {
		return "", err
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	return port, err
}

// kafkaRuntimeProcess is a running Kafka runtime.
type kafkaRuntimeProcess struct {
	cancel   context.CancelFunc
	done     chan struct{} // closed once the process has exited
	err      error         // the process' exit error, set before done is closed
	stopping chan struct{} // closed when stop begins: an exit is then expected
	stopOnce sync.Once
}

// startKafkaRuntimeProcess starts the runtime binary with the given
// environment, its output prefixed into func's own. Canceling ctx stops it.
// While it runs, it is recorded in recordDir (see stopLeftoverKafkaRuntimes);
// an empty recordDir records nothing.
func startKafkaRuntimeProcess(ctx context.Context, bin string, env []string, recordDir string) (*kafkaRuntimeProcess, error) {
	ctx, cancel := context.WithCancel(ctx)
	stdout := newLinePrefixWriter(os.Stdout, kafkaRuntimeLogPrefix)
	stderr := newLinePrefixWriter(os.Stderr, kafkaRuntimeLogPrefix)

	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill() // Interrupt is not implemented on Windows
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = kafkaRuntimeStopTimeout

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting the Kafka runtime: %w", err)
	}
	p := &kafkaRuntimeProcess{
		cancel:   cancel,
		done:     make(chan struct{}),
		stopping: make(chan struct{}),
	}
	record := writeKafkaRuntimeRecord(recordDir, cmd.Process.Pid, cmd.Path)
	go func() {
		p.err = cmd.Wait()
		if record != "" {
			_ = os.Remove(record)
		}
		_ = stdout.Flush()
		_ = stderr.Flush()
		close(p.done)
	}()
	return p, nil
}

// waitReady polls the runtime's readiness endpoint at uri until it reports
// ready, the runtime exits, timeout elapses or ctx is canceled.
func (p *kafkaRuntimeProcess) waitReady(ctx context.Context, uri string, timeout time.Duration, verbose bool) error {
	if verbose {
		fmt.Printf("Waiting for %v\n", uri)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		ok, err := isReady(ctx, uri, timeout, verbose)
		if ok {
			return nil
		}
		if err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-p.done:
			return fmt.Errorf("the Kafka runtime exited before becoming ready (%v); see its %q output above", exitDescription(p.err), strings.TrimSpace(kafkaRuntimeLogPrefix))
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("the Kafka runtime did not become ready within %v: check that run.kafka.brokers is reachable, the topic exists and the credentials are valid", timeout)
			}
			return ErrContextCanceled
		case <-time.After(kafkaRuntimeReadyInterval):
		}
	}
}

// reportExit reports on errs an exit of the runtime which was neither
// requested by stop nor caused by canceling the run (ctx).
func (p *kafkaRuntimeProcess) reportExit(ctx context.Context, errs chan<- error) {
	select {
	case <-p.done:
	case <-p.stopping:
		return
	}
	select {
	case <-p.stopping:
		return
	default:
	}
	if ctx.Err() != nil {
		return
	}
	select {
	case errs <- fmt.Errorf("the Kafka runtime exited unexpectedly (%v)", exitDescription(p.err)):
	case <-p.stopping:
	}
}

// stop stops the runtime, waiting for it to exit. It is safe to call more than
// once.
func (p *kafkaRuntimeProcess) stop() {
	p.stopOnce.Do(func() {
		close(p.stopping)
		p.cancel()
	})
	<-p.done
}

// Left-over runtimes
//
// A func process killed outright (SIGKILL) or crashing gets no chance to stop
// the runtime it started, which then keeps consuming in the background, a
// member of the function's consumer group no one sees. So each runtime is
// recorded in the function's run directory while it runs, and the next
// `func run` of the function stops any whose func process is gone.

// kafkaRuntimeRecord identifies a running Kafka runtime and the func process
// that started it.
type kafkaRuntimeRecord struct {
	PID    int    `json:"pid"`
	Owner  int    `json:"owner"`  // PID of the func process that started it
	Binary string `json:"binary"` // absolute path of the runtime executable
}

// kafkaRuntimeRecordPattern matches the records' file names in a function's
// run directory.
const kafkaRuntimeRecordPattern = "kafka-runtime-*.json"

// writeKafkaRuntimeRecord records the runtime with the given PID, started from
// bin by this process, in dir and returns the record's path, or "" when it
// could not be written (the runtime then simply cannot be cleaned up later).
func writeKafkaRuntimeRecord(dir string, pid int, bin string) string {
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	data, err := json.Marshal(kafkaRuntimeRecord{PID: pid, Owner: os.Getpid(), Binary: bin})
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, strings.Replace(kafkaRuntimeRecordPattern, "*", strconv.Itoa(pid), 1))
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return ""
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ""
	}
	return path
}

// stopLeftoverKafkaRuntimes stops the Kafka runtimes recorded for f whose func
// process is gone, and removes the records of runtimes no longer running. A
// recorded runtime is stopped only if its PID still runs the recorded
// executable, so a PID since reused by another process is never signaled; a
// runtime whose func process is still running (another `func run` of the
// function) is left alone.
func stopLeftoverKafkaRuntimes(f Function) {
	paths, _ := filepath.Glob(filepath.Join(funcJobsDir(f), kafkaRuntimeRecordPattern))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r kafkaRuntimeRecord
		if err := json.Unmarshal(data, &r); err != nil || r.PID <= 1 || r.Owner <= 0 {
			_ = os.Remove(path)
			continue
		}
		if processAlive(r.Owner) {
			continue // still run by its func process
		}
		if exe, err := processExecutable(r.PID); err == nil && samePath(exe, r.Binary) {
			stopProcess(r.PID, kafkaRuntimeStopTimeout)
			fmt.Fprintf(os.Stderr, "Stopped a Kafka runtime (pid %d) left running by an earlier func run.\n", r.PID)
		}
		_ = os.Remove(path)
	}
}

// stopProcess interrupts the process with the given PID, which is not a child
// of this one, and waits up to timeout for it to exit before killing it.
func stopProcess(pid int, timeout time.Duration) {
	_ = interruptProcess(pid)
	if waitProcessExit(pid, timeout) {
		return
	}
	_ = killProcess(pid)
	waitProcessExit(pid, 2*time.Second)
}

// waitProcessExit waits up to timeout for the process with the given PID to
// exit, reporting whether it did.
func waitProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// samePath reports whether a and b name the same file, resolving symbolic
// links (and, on Windows, ignoring case).
func samePath(a, b string) bool {
	if r, err := filepath.EvalSymlinks(a); err == nil {
		a = r
	}
	if r, err := filepath.EvalSymlinks(b); err == nil {
		b = r
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// exitDescription describes a process exit error from exec.Cmd.Wait.
func exitDescription(err error) string {
	if err == nil {
		return "exit status 0"
	}
	return err.Error()
}

// linePrefixWriter writes each complete line written to it to w as a single
// write, prefixed, so lines from concurrent writers to the same w do not
// interleave mid-line. An incomplete trailing line is held until it is
// completed, it reaches kafkaRuntimeMaxLine, or Flush is called.
type linePrefixWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	line   []byte
}

func newLinePrefixWriter(w io.Writer, prefix string) *linePrefixWriter {
	return &linePrefixWriter{w: w, prefix: prefix}
}

func (p *linePrefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(b)
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			p.line = append(p.line, b...)
			if len(p.line) >= kafkaRuntimeMaxLine {
				if err := p.writeLine(); err != nil {
					return 0, err
				}
			}
			break
		}
		p.line = append(p.line, b[:i+1]...)
		if err := p.writeLine(); err != nil {
			return 0, err
		}
		b = b[i+1:]
	}
	return n, nil
}

// Flush writes any buffered incomplete line, newline-terminated.
func (p *linePrefixWriter) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.line) == 0 {
		return nil
	}
	return p.writeLine()
}

// writeLine writes the buffered line, prefixed and newline-terminated, and
// empties the buffer.
func (p *linePrefixWriter) writeLine() error {
	out := make([]byte, 0, len(p.prefix)+len(p.line)+1)
	out = append(out, p.prefix...)
	out = append(out, p.line...)
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	p.line = p.line[:0]
	_, err := p.w.Write(out)
	return err
}
