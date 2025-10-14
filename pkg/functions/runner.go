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
	"time"
)

const (
	defaultRunHost        = "127.0.0.1" // TODO allow to be altered via a runOpt
	defaultRunPort        = "8080"
	defaultRunDialTimeout = 2 * time.Second
	defaultRunStopTimeout = 10 * time.Second
	readinessEndpoint     = "/health/readiness"
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

	if address != "" {
		var err error
		host, port, err = net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid address format '%s': %w", address, err)
		}
	}

	port, err = choosePort(host, port)
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

	// Wait for it to become available before returning the metadata.
	err = waitFor(ctx, job, startTimeout)
	return
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
	case "java":
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

	// cmd.Cancel = stop // TODO: use when we upgrade to go 1.20

	// See the 1.19 [release notes](https://tip.golang.org/doc/go1.19) which state:
	//   A Cmd with a non-empty Dir field and nil Env now implicitly sets the PWD environment variable for the subprocess to match Dir.
	//   The new method Cmd.Environ reports the environment that would be used to run the command, including the implicitly set PWD variable.
	// cmd.Env = append(cmd.Environ(), "PORT="+job.Port) // requires go 1.19
	cmd.Env = append(cmd.Env, "LISTEN_ADDRESS="+net.JoinHostPort(job.Host, job.Port), "PWD="+cmd.Dir)

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

	// See 1.19 [release notes](https://tip.golang.org/doc/go1.19) which state:
	//   A Cmd with a non-empty Dir field and nil Env now implicitly sets the
	//   PWD environment variable for the subprocess to match Dir.
	//   The new method Cmd.Environ reports the environment that would be used
	//   to run the command, including the implicitly set PWD variable.
	cmd.Env = append(cmd.Env, "PORT="+job.Port, "LISTEN_ADDRESS="+listenAddress, "PWD="+cmd.Dir)

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

func waitFor(ctx context.Context, job *Job, timeout time.Duration) error {
	var (
		uri      = fmt.Sprintf("http://%s%s", net.JoinHostPort(job.Host, job.Port), readinessEndpoint)
		interval = 500 * time.Millisecond
	)

	if job.verbose {
		fmt.Printf("Waiting for %v\n", uri)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		ok, err := isReady(ctx, uri, timeout, job.verbose)
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

// choosePort returns an unused port on the given interface (host)
// Note this is not fool-proof becase of a race with any other processes
// looking for a port at the same time.  If that is important, we can implement
// a check-lock-check via the filesystem.
// Also note that TCP is presumed.
func choosePort(iface, preferredPort string) (string, error) {
	var (
		port = preferredPort
		l    net.Listener
		err  error
	)

	// Try preferred
	if l, err = net.Listen("tcp", net.JoinHostPort(iface, port)); err == nil {
		l.Close() // note err==nil
		return port, nil
	}

	// OS-chosen
	if l, err = net.Listen("tcp", net.JoinHostPort(iface, "")); err != nil {
		return "", fmt.Errorf("cannot bind tcp: %w", err)
	}
	l.Close() // begins aforementioned race
	if _, port, err = net.SplitHostPort(l.Addr().String()); err != nil {
		return "", fmt.Errorf("cannot parse port: %w", err)
	}
	return port, nil
}

func pythonCmd() string {
	_, err := exec.LookPath("python")
	if err != nil {
		return "python3"
	}
	return "python"
}
