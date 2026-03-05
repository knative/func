package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	mobyContainer "github.com/moby/moby/api/types/container"
	mobyNetwork "github.com/moby/moby/api/types/network"
	mobyClient "github.com/moby/moby/client"
	"github.com/pkg/errors"

	fn "knative.dev/func/pkg/functions"
)

const (
	// DefaultHost is the standard ipv4 loopback
	DefaultHost = "127.0.0.1"

	// DefaultPort is used as the preferred port, and in the unlikly event of an
	// error querying the OS for a free port during allocation.
	DefaultPort = "8080"

	// DefaultDialTimeout when checking if a port is available.
	DefaultDialTimeout = 2 * time.Second

	// DefaultStopTimeout when attempting to stop underlying containers.
	DefaultStopTimeout = 10 * time.Second
)

type ErrNoImage struct{}

func (e ErrNoImage) Error() string {
	return "Function has no associated image. Has it been built?"
}

// Runner starts and stops functions as local containers.
type Runner struct {
	verbose bool // Verbose logging
	out     io.Writer
	errOut  io.Writer
}

// NewRunner creates an instance of a docker-backed runner.
func NewRunner(verbose bool, out, errOut io.Writer) *Runner {
	return &Runner{
		verbose: verbose,
		out:     out,
		errOut:  errOut,
	}
}

// Run the function.
func (n *Runner) Run(ctx context.Context, f fn.Function, address string, startTimeout time.Duration) (job *fn.Job, err error) {

	var (
		host = DefaultHost
		port = DefaultPort
		c    mobyClient.APIClient // Docker client
		id   string               // ID of running container
		conn net.Conn             // Connection to container's stdio

		// Channels for gathering runtime errors from the container instance
		copyErrCh    = make(chan error, 10)
		runtimeErrCh = make(chan error, 10)
	)

	// Parse address if provided
	if address != "" {
		var err error
		host, port, err = net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid address format '%s': %w", address, err)
		}
	}

	// Choose an available port
	port = choosePort(host, port, DefaultDialTimeout)

	if f.Build.Image == "" {
		return job, ErrNoImage{}
	}
	if c, _, err = NewClient(mobyClient.DefaultDockerHost); err != nil {
		return job, errors.Wrap(err, "failed to create Docker API client")
	}
	if id, err = newContainer(ctx, c, f, host, port, n.verbose); err != nil {
		return job, errors.Wrap(err, "runner unable to create container")
	}
	if conn, err = copyStdio(ctx, c, id, copyErrCh, n.out, n.errOut); err != nil {
		return
	}

	// Wait for errors premature exits
	waitResult := c.ContainerWait(ctx, id, mobyClient.ContainerWaitOptions{
		Condition: mobyContainer.WaitConditionNextExit,
	})
	go func() {
		for {
			select {
			case err = <-copyErrCh:
				runtimeErrCh <- err
			case body := <-waitResult.Result:
				// NOTE: currently an exit is not expected and thus a return, for any
				// reason, is considered an error even when the exit code is 0.
				// Functions are expected to be long-running processes that do not exit
				// of their own accord when run locally.  Should this expectation
				// change in the future, this channel-based wait may need to be
				// expanded to accept the case of a voluntary, successful exit.
				runtimeErrCh <- fmt.Errorf("exited code %v", body.StatusCode)
			case err = <-waitResult.Error:
				runtimeErrCh <- err
			}
		}
	}()

	if _, err = c.ContainerStart(ctx, id, mobyClient.ContainerStartOptions{}); err != nil {
		return job, errors.Wrap(err, "runner unable to start container")
	}

	// Stopper
	stop := func() error {
		var (
			timeout = DefaultStopTimeout
			ctx     = context.Background()
		)
		timeoutSecs := int(timeout.Seconds())
		if _, err = c.ContainerStop(ctx, id, mobyClient.ContainerStopOptions{
			Timeout: &timeoutSecs,
		}); err != nil {
			return fmt.Errorf("error stopping container %v: %v", id, err)
		}
		if _, err = c.ContainerRemove(ctx, id, mobyClient.ContainerRemoveOptions{}); err != nil {
			return fmt.Errorf("error removing container %v: %v", id, err)
		}
		if err = conn.Close(); err != nil {
			return fmt.Errorf("error closing connection to container: %v", err)
		}
		if err = c.Close(); err != nil {
			return fmt.Errorf("error closing daemon client: %v", err)
		}
		return nil
	}

	if startTimeout > 0 {
		startCtx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()

		readyCh := make(chan struct{})
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				if err := dial(host, port, DefaultDialTimeout); err == nil {
					select {
					case readyCh <- struct{}{}:
					default:
					}
					return
				}
				select {
				case <-startCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()

		select {
		case <-readyCh:

		case err := <-runtimeErrCh:
			_ = stop()
			return nil, fmt.Errorf("container error before readiness: %w", err)
		case <-startCtx.Done():
			_ = stop()
			return nil, fmt.Errorf("timeout waiting for function to start")
		}

	}

	// Job reporting port, runtime errors and provides a mechanism for stopping.
	return fn.NewJob(f, host, port, runtimeErrCh, stop, n.verbose)
}

// Dial the given (tcp) port on the given interface, returning an error if it is
// unreachable.
func dial(host, port string, dialTimeout time.Duration) (err error) {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		return
	}
	defer conn.Close()
	return
}

// choosePort returns an unused port
// Note this is not fool-proof because of a race with any other processes
// looking for a port at the same time.
// Note that TCP is presumed.
func choosePort(host, preferredPort string, dialTimeout time.Duration) string {
	// If we can not dial the preferredPort, it is assumed to be open.
	if err := dial(host, preferredPort, dialTimeout); err != nil {
		return preferredPort
	}

	// Use an OS-chosen port
	lis, err := net.Listen("tcp", net.JoinHostPort(host, "")) // listen on any open port
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to check for open ports. using fallback %v. %v", DefaultPort, err)
		return DefaultPort
	}
	defer lis.Close()

	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to extract port from allocated listener address '%v'. %v", lis.Addr(), err)
		return DefaultPort
	}
	return port

}

func newContainer(ctx context.Context, c mobyClient.APIClient, f fn.Function, host, port string, verbose bool) (id string, err error) {
	var (
		containerCfg mobyContainer.Config
		hostCfg      mobyContainer.HostConfig
	)
	if containerCfg, err = newContainerConfig(f, port, verbose); err != nil {
		return
	}
	if hostCfg, err = newHostConfig(host, port); err != nil {
		return
	}
	t, err := c.ContainerCreate(ctx, mobyClient.ContainerCreateOptions{
		Config:     &containerCfg,
		HostConfig: &hostCfg,
	})
	if err != nil {
		return
	}
	return t.ID, nil
}

func newContainerConfig(f fn.Function, _ string, verbose bool) (c mobyContainer.Config, err error) {
	httpPort := mobyNetwork.MustParsePort("8080/tcp")
	c = mobyContainer.Config{
		Image:        f.Build.Image,
		Tty:          false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  false,
		ExposedPorts: mobyNetwork.PortSet{httpPort: {}},
	}

	// Environment Variables
	// Interpolate references to local environment variables and convert to a
	// simple string slice for use with container.Config
	envs, err := fn.Interpolate(f.Run.Envs)
	if err != nil {
		return
	}
	for k, v := range envs {
		c.Env = append(c.Env, k+"="+v)
	}
	if verbose {
		c.Env = append(c.Env, "VERBOSE=true")
	}

	return
}

func newHostConfig(host, port string) (c mobyContainer.HostConfig, err error) {
	httpPort := mobyNetwork.MustParsePort("8080/tcp")
	hostAddr, err := netip.ParseAddr(host)
	if err != nil {
		return c, fmt.Errorf("invalid host address %q: %w", host, err)
	}
	ports := mobyNetwork.PortMap{
		httpPort: {
			mobyNetwork.PortBinding{
				HostPort: port,
				HostIP:   hostAddr,
			},
		},
	}
	return mobyContainer.HostConfig{PortBindings: ports}, nil
}

// copy stdin and stdout from the container of the given ID.  Errors encountered
// during copy are communicated via a provided errs channel.
func copyStdio(ctx context.Context, c mobyClient.APIClient, id string, errs chan error, out, errOut io.Writer) (conn net.Conn, err error) {
	res, err := c.ContainerAttach(ctx, id, mobyClient.ContainerAttachOptions{
		Stdout: true,
		Stderr: true,
		Stdin:  false,
		Stream: true,
	})
	if err != nil {
		return conn, errors.Wrap(err, "runner unable to attach to container's stdio")
	}
	go func() {
		_, err := stdcopy.StdCopy(out, errOut, res.Reader)
		errs <- err
	}()
	return res.Conn, nil
}
