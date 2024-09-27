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

func (r *defaultRunner) Run(ctx context.Context, f Function, startTimeout time.Duration) (job *Job, err error) {
	var (
		port    string
		runFn   func() error
		verbose = r.client.verbose
	)

	port, err = choosePort(defaultRunHost, defaultRunPort)
	if err != nil {
		return nil, fmt.Errorf("cannot choose port: %w", err)
	}

	// Job contains metadata and references for the running function.
	job, err = NewJob(f, defaultRunHost, port, nil, nil, verbose)
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
		err = ErrRunnerNotImplemented{runtime}
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
	// BUILD
	// -----
	// TODO: extract the build command code from the OCI Container Builder
	// and have both the runner and OCI Container Builder use the same here.
	if job.verbose {
		fmt.Printf("cd %v && go build -o f.bin\n", job.Dir())
	}

	// Build
	args := []string{"build", "-o", "f.bin"}
	if job.verbose {
		args = append(args, "-v")
	}
	cmd := exec.CommandContext(ctx, "go", args...)
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
	//  TODO: Update the functions go runtime to accept LISTEN_ADDRESS rather
	// than just port in able to allow listening on other interfaces
	// (keeping the default localhost only)
	if job.Host != "127.0.0.1" {
		fmt.Fprintf(os.Stderr, "Warning: the Go functions runtime currently only supports localhost '127.0.0.1'.  Requested listen interface '%v' will be ignored.", job.Host)
	}
	// See the 1.19 [release notes](https://tip.golang.org/doc/go1.19) which state:
	//   A Cmd with a non-empty Dir field and nil Env now implicitly sets the PWD environment variable for the subprocess to match Dir.
	//   The new method Cmd.Environ reports the environment that would be used to run the command, including the implicitly set PWD variable.
	// cmd.Env = append(cmd.Environ(), "PORT="+job.Port) // requires go 1.19
	cmd.Env = append(cmd.Env, "PORT="+job.Port, "PWD="+cmd.Dir)

	// Running asynchronously allows for the client Run method to return
	// metadata about the running function such as its chosen port.
	go func() {
		job.Errors <- cmd.Run()
	}()
	return
}

func waitFor(ctx context.Context, job *Job, timeout time.Duration) error {
	var (
		uri      = fmt.Sprintf("http://%s:%s%s", job.Host, job.Port, readinessEndpoint)
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
