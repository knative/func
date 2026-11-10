package functions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"knative.dev/func/pkg/kafkaruntime"
)

const (
	defaultRunHost    = "127.0.0.1"
	defaultRunPort    = "8080"
	readinessEndpoint = "/health/readiness"
)

type defaultRunner struct {
	client *Client
	out    io.Writer
	err    io.Writer
}

func newDefaultRunner(client *Client, out, err io.Writer) *defaultRunner {
	return &defaultRunner{
		client: client,
		out:    out,
		err:    err,
	}
}

func (r *defaultRunner) Run(ctx context.Context, f Function, address string, startTimeout time.Duration) (job *Job, err error) {
	var (
		runFn   func() error
		verbose = r.client.verbose
	)

	// Parse address if provided, otherwise use defaults
	host := defaultRunHost
	port := defaultRunPort
	explicitPort := address != ""

	if address != "" {
		var err error
		host, port, err = net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid address format '%s': %w", address, err)
		}
	}

	port, err = choosePort(host, port, explicitPort)
	if err != nil {
		return nil, fmt.Errorf("cannot choose port: %w", err)
	}

	// Job contains metadata and references for the running function.
	job, err = NewJob(f, host, port, nil, nil, verbose)
	if err != nil {
		return
	}

	// Scaffold the function such that it can be run.
	if err = r.client.Scaffold(ctx, f, job.Dir()); err != nil {
		return
	}

	// Runner for the Function's runtime.
	if runFn, err = getRunFunc(ctx, job); err != nil {
		return
	}

	// Run the scaffolded function asynchronously.
	if err = runFn(); err != nil {
		return
	}

	// Start the Kafka runtime adapter as a sub-process when run.kafka is
	// configured. It consumes Kafka and delivers each record to the function
	// over loopback HTTP -- the local counterpart of the on-cluster sidecar.
	kafkaReadinessURI, err := startKafkaAdapter(ctx, job)
	if err != nil {
		return
	}

	// Wait for the function to become available before returning the metadata.
	if err = waitFor(ctx, job, startTimeout); err != nil {
		return
	}

	// Gate on the adapter too: its readiness goes green only once the consumer
	// has connected and been assigned partitions, so a bad broker or bad
	// credentials fails `func run` fast here rather than silently delivering
	// nothing.
	if kafkaReadinessURI != "" {
		err = waitForURI(ctx, kafkaReadinessURI, startTimeout, job.verbose)
	}
	return
}

// startKafkaAdapter runs the language-agnostic Kafka runtime adapter as a child
// process next to the function when run.kafka is configured. The adapter
// consumes Kafka and POSTs each record to the function over loopback HTTP,
// committing only on a 2xx -- the same delivery contract as the on-cluster
// sidecar, so local `func run` works for every language with no in-process
// code. It is wired to the run context (torn down on Ctrl-C) and feeds the same
// job.Errors channel, so an adapter crash surfaces like a function crash.
//
// It returns the adapter's readiness URI (empty when no adapter was started) so
// the caller can gate startup on the consumer connecting.
func startKafkaAdapter(ctx context.Context, job *Job) (readinessURI string, err error) {
	k := job.Function.Run.Kafka
	if k == nil || k.Brokers == "" || k.Topic == "" || k.ConsumerGroup == "" {
		return "", nil // no Kafka configured
	}

	bin, err := kafkaruntime.Extract()
	if errors.Is(err, kafkaruntime.ErrUnavailable) {
		fmt.Fprintf(os.Stderr, "[kafka] local Kafka runtime unavailable on this platform (%s/%s); records will not be delivered locally\n", runtime.GOOS, runtime.GOARCH)
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("extracting kafka runtime adapter: %w", err)
	}

	// Pick a free loopback port for the adapter's health server, distinct from
	// the function's port.
	healthPort, err := freeLoopbackPort()
	if err != nil {
		return "", fmt.Errorf("choosing kafka adapter port: %w", err)
	}
	listenAddr := net.JoinHostPort(defaultRunHost, healthPort)

	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = kafkaAdapterEnv(job, listenAddr)
	cmd.Stdout = newPrefixWriter(os.Stdout, "[kafka] ")
	cmd.Stderr = newPrefixWriter(os.Stderr, "[kafka] ")
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 5 * time.Second

	if job.verbose {
		fmt.Printf("Starting kafka runtime adapter: %v (health %v, target http://%v/)\n",
			bin, listenAddr, net.JoinHostPort(job.Host, job.Port))
	}

	// Running asynchronously mirrors the function child; the adapter self-waits
	// for FUNCTION_TARGET, so starting it after the function is a harmless race.
	go func() {
		job.Errors <- cmd.Run()
	}()

	return fmt.Sprintf("http://%s%s", listenAddr, readinessEndpoint), nil
}

// kafkaAdapterEnv builds the environment for the Kafka runtime adapter: the
// parent environment, the target/health wiring, and the KAFKA_* config relocated
// from the function (the function no longer consumes Kafka in-process).
//
// TODO(wip): run the credential values through Interpolate (as buildRunnerEnv
// does for Run.Envs) so {{ env:NAME }} resolves locally, and warn on an
// unresolved cluster {{ secret:... }} reference. Kept as raw passthrough here to
// match the pre-relocation behavior exactly.
func kafkaAdapterEnv(job *Job, listenAddr string) []string {
	k := job.Function.Run.Kafka
	env := os.Environ()
	env = append(env,
		"FUNCTION_TARGET=http://"+net.JoinHostPort(job.Host, job.Port)+"/",
		"LISTEN_ADDRESS="+listenAddr,
		"KAFKA_BROKERS="+k.Brokers,
		"KAFKA_TOPIC="+k.Topic,
		"KAFKA_CONSUMER_GROUP="+k.ConsumerGroup,
	)
	if k.SecurityProtocol != "" {
		env = append(env, "KAFKA_SECURITY_PROTOCOL="+k.SecurityProtocol)
	}
	if k.TLS != nil {
		if k.TLS.CACert != "" {
			env = append(env, "KAFKA_TLS_CA_CERT="+k.TLS.CACert)
		}
		if k.TLS.ClientCert != "" {
			env = append(env, "KAFKA_TLS_CLIENT_CERT="+k.TLS.ClientCert)
		}
		if k.TLS.ClientKey != "" {
			env = append(env, "KAFKA_TLS_CLIENT_KEY="+k.TLS.ClientKey)
		}
		if k.TLS.SkipVerify {
			env = append(env, "KAFKA_TLS_SKIP_VERIFY=true")
		} else {
			env = append(env, "KAFKA_TLS_SKIP_VERIFY=false")
		}
	}
	if k.SASL != nil {
		env = append(env, "KAFKA_SASL_MECHANISM="+k.SASL.EffectiveMechanism())
		if k.SASL.User != "" {
			env = append(env, "KAFKA_SASL_USER="+k.SASL.User)
		}
		if k.SASL.Password != "" {
			env = append(env, "KAFKA_SASL_PASSWORD="+k.SASL.Password)
		}
	}
	return env
}

// prefixWriter prefixes every line written to it, so interleaved adapter output
// is distinguishable from the function's in `func run` logs.
type prefixWriter struct {
	w      io.Writer
	prefix string
	atBOL  bool
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, atBOL: true}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	for _, c := range b {
		if p.atBOL {
			if _, err := io.WriteString(p.w, p.prefix); err != nil {
				return 0, err
			}
			p.atBOL = false
		}
		if _, err := p.w.Write([]byte{c}); err != nil {
			return 0, err
		}
		p.atBOL = c == '\n'
	}
	return len(b), nil
}

// getRunFunc returns a function which will run the user's Function based on
// the jobs runtime.
func getRunFunc(ctx context.Context, job *Job) (runFn func() error, err error) {
	runtime := job.Function.Runtime
	switch runtime {
	case "":
		err = ErrRuntimeRequired
	case "go":
		runFn = func() error { return runGo(ctx, job) }
	case "python":
		runFn = func() error { return runPython(ctx, job) }
	case "springboot":
		err = ErrRunnerNotImplemented{runtime}
	case "node":
		err = ErrRunnerNotImplemented{runtime}
	case "typescript":
		err = ErrRunnerNotImplemented{runtime}
	case "rust":
		err = ErrRunnerNotImplemented{runtime}
	case "quarkus":
		err = ErrRunnerNotImplemented{runtime}
	default:
		err = ErrRuntimeNotRecognized{runtime}
	}
	return
}

func runGo(ctx context.Context, job *Job) (err error) {
	// TODO:  long-term, the correct architecture is to not read env vars
	// from deep within a package, but rather to expose the setting as a
	// variable and leave interacting with the environment to main.
	// This is a shortcut used by many packages, however, so it will work for
	// now.
	gobin := os.Getenv("FUNC_GO") // Use if provided
	if gobin == "" {
		gobin = "go" // default to looking on PATH
	}

	// BUILD
	// -----
	// TODO: extract the build command code from the OCI Container Builder
	// and have both the runner and OCI Container Builder use the same here.
	if job.verbose {
		fmt.Printf("cd %v && go build -o f.bin\n", job.Dir())
	}

	args := []string{"mod", "tidy"}
	if job.verbose {
		args = append(args, "-v")
	}
	cmd := exec.CommandContext(ctx, gobin, args...)
	cmd.Dir = job.Dir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return
	}

	// Build
	args = []string{"build", "-o", "f.bin"}
	if job.verbose {
		args = append(args, "-v")
	}

	cmd = exec.CommandContext(ctx, gobin, args...)
	cmd.Dir = job.Dir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return
	}

	// Run
	// ---
	bin := filepath.Join(job.Dir(), "f.bin")
	if job.verbose {
		fmt.Printf("cd %v && PORT=%v %v\n", job.Function.Root, job.Port, bin)
	}
	cmd = exec.CommandContext(ctx, bin)
	cmd.Dir = job.Function.Root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			// Interrupt is not implemented on windows apparently
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	// force kill after delay if Interrupt signal did not work to not hang indefinitely
	cmd.WaitDelay = 5 * time.Second

	cmd.Env, err = buildRunnerEnv(job, map[string]string{
		"LISTEN_ADDRESS": net.JoinHostPort(job.Host, job.Port),
		"PWD":            cmd.Dir,
	})
	if err != nil {
		return fmt.Errorf("error building runner environment: %w", err)
	}

	// Running asynchronously allows for the client Run method to return
	// metadata about the running function such as its chosen port.
	go func() {
		job.Errors <- cmd.Run()
	}()
	return
}

func runPython(ctx context.Context, job *Job) (err error) {
	if job.verbose {
		fmt.Printf("cd %v\n", job.Dir())
	}

	// Create venv
	if job.verbose {
		fmt.Printf("python -m venv .venv\n")
	}
	cmd := exec.CommandContext(ctx, pythonCmd(), "-m", "venv", ".venv")
	cmd.Dir = job.Dir()
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err = cmd.Run(); err != nil {
		return
	}

	// Upgrade pip
	// Unlikely to be necessary in the majority of cases, and adds a nontrivial
	// latency to the run process, upgrading pip is therefore disabled by
	// default but can be enabled by setting "upgrade-pip" to "true" in the
	// context.  For example, adding a flag --upgrade-pip to the CLI which adds
	// the key to the context used by client.Run.
	if upgrade, ok := ctx.Value("upgrade-pip").(bool); ok && upgrade {
		if job.verbose {
			fmt.Printf("./.venv/bin/pip install --upgrade pip\n")
		}
		cmd = exec.CommandContext(ctx, "./.venv/bin/pip", "install", "--upgrade", "pip")
		cmd.Dir = job.Dir()
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err = cmd.Run(); err != nil {
			return
		}
	}

	// Install  dependencies
	if job.verbose {
		fmt.Printf("./.venv/bin/pip install .\n")
	}
	cmd = exec.CommandContext(ctx, "./.venv/bin/pip", "install", ".")
	cmd.Dir = job.Dir()
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err = cmd.Run(); err != nil {
		return
	}

	// Run
	listenAddress := net.JoinHostPort(job.Host, job.Port)
	if job.verbose {
		fmt.Printf("PORT=%v LISTEN_ADDRESS=%v ./.venv/bin/python ./service/main.py\n", job.Port, listenAddress)
	}
	cmd = exec.CommandContext(ctx, "./.venv/bin/python", "./service/main.py")
	// cmd.Dir = job.Function.Root // handled by the middleware
	cmd.Dir = job.Dir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 5 * time.Second

	cmd.Env, err = buildRunnerEnv(job, map[string]string{
		"PORT":           job.Port,
		"LISTEN_ADDRESS": listenAddress,
		"PWD":            cmd.Dir,
	})
	if err != nil {
		return fmt.Errorf("error building runner environment: %w", err)
	}

	// Running asynchronously allows for the client Run method to return
	// metadata about the running function such as its chosen port.
	go func() {
		job.Errors <- cmd.Run()
	}()

	// TODO(enhancement):  context cancellation such that we can both
	// signal the running command process to complete (thus triggering the
	// .Stop lifecycle handling event) and allow the following cleanup task
	// to be run. For now just wait a moment and then immediately clean up...
	// creating a racing condition.

	return
}

// buildRunnerEnv constructs the environment for a host-run subprocess.
// It starts with the parent process environment (os.Environ), layers on the
// provided extras (e.g. PORT, LISTEN_ADDRESS, PWD), and then applies any
// environment variables defined in func.yaml or passed via -e flags.
func buildRunnerEnv(job *Job, extras map[string]string) ([]string, error) {
	env := os.Environ()

	for k, v := range extras {
		env = append(env, k+"="+v)
	}

	// Interpolate and append env vars from func.yaml / -e flags.
	funcEnvs, err := Interpolate(job.Function.Run.Envs)
	if err != nil {
		return nil, fmt.Errorf("error interpolating environment variables: %w", err)
	}
	for k, v := range funcEnvs {
		env = append(env, k+"="+v)
	}

	// NOTE: Kafka delivery no longer runs in-process inside the function. The
	// KAFKA_* config is set on the Kafka runtime adapter sub-process instead
	// (see kafkaAdapterEnv / startKafkaAdapter), and FUNC_TRANSPORT is gone.

	return env, nil
}

func waitFor(ctx context.Context, job *Job, timeout time.Duration) error {
	uri := fmt.Sprintf("http://%s%s", net.JoinHostPort(job.Host, job.Port), readinessEndpoint)
	return waitForURI(ctx, uri, timeout, job.verbose)
}

// waitForURI polls uri's readiness endpoint until it returns 200, the timeout
// elapses, or the context is canceled. It is used for both the function and the
// Kafka runtime adapter.
func waitForURI(ctx context.Context, uri string, timeout time.Duration, verbose bool) error {
	interval := 500 * time.Millisecond

	if verbose {
		fmt.Printf("Waiting for %v\n", uri)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		ok, err := isReady(ctx, uri, timeout, verbose)
		if ok || err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrRunTimeout{timeout}
			}
			return ErrContextCanceled
		case <-time.After(interval):
			continue
		}
	}
}

// isReady returns true if the uri could be reached and returned an HTTP 200.
// False is returned if a nonfatal error was encountered (which will have been
// printed to stderr), and an error is returned when an error is encountered
// that is unlikely to be due to startup (malformed requests).
func isReady(ctx context.Context, uri string, timeout time.Duration, verbose bool) (ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return false, fmt.Errorf("error creating readiness check context. %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return false, ErrRunTimeout{timeout}
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "endpoint not available. %v\n", err)
		}
		return false, nil // nonfatal.  May still be starting up.
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		if verbose {
			fmt.Fprintf(os.Stderr, "endpoint returned HTTP %v:\n", res.StatusCode)
			dump, _ := httputil.DumpResponse(res, true)
			fmt.Println(string(dump))
		}
		return false, nil // nonfatal.  May still be starting up
	}

	return true, nil
}

// choosePort returns an unused port on the given interface (host).
// If explicitPort is true and the preferred port cannot be bound, an error is returned.
// If explicitPort is false (default port), it falls back to an OS-chosen port if the preferred port is unavailable.
// Note this is not fool-proof because of a race with any other processes
// looking for a port at the same time.  If that is important, we can implement
// a check-lock-check via the filesystem.
// Also note that TCP is presumed.
func choosePort(iface, preferredPort string, explicitPort bool) (string, error) {
	var (
		port = preferredPort
		l    net.Listener
		err  error
	)

	// Try preferred port
	if l, err = net.Listen("tcp", net.JoinHostPort(iface, port)); err == nil {
		l.Close()
		return port, nil
	}

	// If user explicitly provided a port and it's unavailable, return typed error
	if explicitPort {
		return "", &ErrPortUnavailableError{
			Port: port,
			Err:  err,
		}
	}

	// For default ports, fall back to OS-chosen port
	if l, err = net.Listen("tcp", net.JoinHostPort(iface, "")); err != nil {
		return "", fmt.Errorf("cannot bind tcp: %w", err)
	}
	l.Close() // begins aforementioned race
	if _, port, err = net.SplitHostPort(l.Addr().String()); err != nil {
		return "", fmt.Errorf("cannot parse port: %w", err)
	}
	return port, nil
}

// freeLoopbackPort returns an OS-chosen free port on the loopback interface.
// Unlike choosePort, it resolves the actual bound port (choosePort returns the
// requested value verbatim when that value binds successfully). There is an
// inherent close-then-reuse race, acceptable here as the adapter binds right
// after.
func freeLoopbackPort() (string, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(defaultRunHost, "0"))
	if err != nil {
		return "", err
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	return port, err
}

func pythonCmd() string {
	_, err := exec.LookPath("python")
	if err != nil {
		return "python3"
	}
	return "python"
}
