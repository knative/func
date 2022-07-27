package docker

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"

	fn "knative.dev/kn-plugin-func"
)

const (
	// DefaultHost is the standard ipv4 looback
	DefaultHost = "127.0.0.1"

	// DefaultPort is used as the preferred port, and in the unlikly event of an
	// error querying the OS for a free port during allocation.
	DefaultPort = "8080"

	// DefaultDialTimeout when checking if a port is available.
	DefaultDialTimeout = 2 * time.Second

	// DefaultStopTimeout when attempting to stop underlying containers.
	DefaultStopTimeout = 10 * time.Second
)

// Runner starts and stops functions as local containers.
type Runner struct {
	verbose bool // Verbose logging
}

// NewRunner creates an instance of a docker-backed runner.
func NewRunner(verbose bool) *Runner {
	return &Runner{verbose: verbose}
}

// Run the function.
func (n *Runner) Run(ctx context.Context, f fn.Function) (job *fn.Job, err error) {

	var (
		port = choosePort(DefaultHost, DefaultPort, DefaultDialTimeout)
		c    client.CommonAPIClient // Docker client
		id   string                 // ID of running container
		conn net.Conn               // Connection to container's stdio

		// Channels for gathering runtime errors from the container instance
		copyErrCh  = make(chan error, 10)
		contBodyCh <-chan container.ContainerWaitOKBody
		contErrCh  <-chan error

		// Combined runtime error channel for sending all errors to caller
		runtimeErrCh = make(chan error, 10)
	)

	if f.Image == "" {
		return job, errors.New("Function has no associated image. Has it been built?")
	}
	if c, _, err = NewClient(client.DefaultDockerHost); err != nil {
		return job, errors.Wrap(err, "failed to create Docker API client")
	}
	if id, err = newContainer(ctx, c, f, port, n.verbose); err != nil {
		return job, errors.Wrap(err, "runner unable to create container")
	}
	if conn, err = copyStdio(ctx, c, id, copyErrCh); err != nil {
		return
	}

	// Wait for errors or premature exits
	contBodyCh, contErrCh = c.ContainerWait(ctx, id, container.WaitConditionNextExit)
	go func() {
		for {
			select {
			case err = <-copyErrCh:
				runtimeErrCh <- err
			case body := <-contBodyCh:
				// NOTE: currently an exit is not expected and thus a return, for any
				// reason, is considered an error even when the exit code is 0.
				// Functions are expected to be long-running processes that do not exit
				// of their own accord when run locally.  Should this expectation
				// change in the future, this channel-based wait may need to be
				// expanded to accept the case of a voluntary, successful exit.
				runtimeErrCh <- fmt.Errorf("exited code %v", body.StatusCode)
			case err = <-contErrCh:
				runtimeErrCh <- err
			}
		}
	}()

	// Start
	if err = c.ContainerStart(ctx, id, types.ContainerStartOptions{}); err != nil {
		return job, errors.Wrap(err, "runner unable to start container")
	}

	// Stopper
	stop := func() {
		var (
			timeout = DefaultStopTimeout
			ctx     = context.Background()
		)
		if err = c.ContainerStop(ctx, id, &timeout); err != nil {
			fmt.Fprintf(os.Stderr, "error stopping container %v: %v\n", id, err)
		}
		if err = c.ContainerRemove(ctx, id, types.ContainerRemoveOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "error removing container %v: %v\n", id, err)
		}
		if err = conn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error closing connection to container: %v\n", err)
		}
		if err = c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error closing daemon client: %v\n", err)
		}
	}

	// Job reporting port, runtime errors and provides a mechanism for stopping.
	return fn.NewJob(f, port, runtimeErrCh, stop)
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
// Note this is not fool-proof becase of a race with any other processes
// looking for a port at the same time.
// Note that TCP is presumed.
func choosePort(host string, preferredPort string, dialTimeout time.Duration) string {
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

func newContainer(ctx context.Context, c client.CommonAPIClient, f fn.Function, port string, verbose bool) (id string, err error) {
	var (
		containerCfg container.Config
		hostCfg      container.HostConfig
	)
	if containerCfg, err = newContainerConfig(f, port, verbose); err != nil {
		return
	}
	if hostCfg, err = newHostConfig(port); err != nil {
		return
	}
	t, err := c.ContainerCreate(ctx, &containerCfg, &hostCfg, nil, nil, "")
	if err != nil {
		return
	}
	return t.ID, nil
}

func newContainerConfig(f fn.Function, _ string, verbose bool) (c container.Config, err error) {
	// httpPort := nat.Port(fmt.Sprintf("%v/tcp", port))
	httpPort := nat.Port("8080/tcp")
	c = container.Config{
		Image:        f.Image,
		Tty:          false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  false,
		ExposedPorts: map[nat.Port]struct{}{httpPort: {}},
	}

	// Environment Variables
	// Interpolate references to local environment variables and convert to a
	// simple string slice for use with container.Config
	envs, err := fn.Interpolate(f.Envs)
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

func newHostConfig(port string) (c container.HostConfig, err error) {
	// httpPort := nat.Port(fmt.Sprintf("%v/tcp", port))
	httpPort := nat.Port("8080/tcp")
	ports := map[nat.Port][]nat.PortBinding{
		httpPort: {
			nat.PortBinding{
				HostPort: port,
				HostIP:   "127.0.0.1",
			},
		},
	}
	return container.HostConfig{PortBindings: ports}, nil
}

// copy stdin and stdout from the container of the given ID.  Errors encountered
// during copy are communicated via a provided errs channel.
func copyStdio(ctx context.Context, c client.CommonAPIClient, id string, errs chan error) (conn net.Conn, err error) {
	var (
		res types.HijackedResponse
		opt = types.ContainerAttachOptions{
			Stdout: true,
			Stderr: true,
			Stdin:  false,
			Stream: true,
		}
	)
	if res, err = c.ContainerAttach(ctx, id, opt); err != nil {
		return conn, errors.Wrap(err, "runner unable to attach to container's stdio")
	}
	go func() {
		_, err := stdcopy.StdCopy(os.Stdout, os.Stderr, res.Reader)
		errs <- err
	}()
	return res.Conn, nil
}
