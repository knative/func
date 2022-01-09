package docker

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
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

// Runner starts and stops Functions as local contaieners.
type Runner struct {
	// Verbose logging
	Verbose bool
}

// NewRunner creates an instance of a docker-backed runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run the Function.
func (n *Runner) Run(ctx context.Context, f fn.Function) (port string, stop func() error, runtimeErrCh chan error, err error) {
	// Ensure image is built before running
	if f.Image == "" {
		err = errors.New("Function has no associate image. Has it been built?")
		return
	}

	// Choose a free port on the given interface, with the given default
	// to use if available.  Limit querying to the given timeout.
	port = choosePort(DefaultHost, DefaultPort, DefaultDialTimeout)

	// Container config
	containerCfg, err := newContainerConfig(f, port, n.Verbose)
	if err != nil {
		return
	}

	// Host config
	hostCfg, err := newHostConfig(port)
	if err != nil {
		return
	}

	// Create the job running asynchronously
	// Caller's reponsibility to stop() either on context cancelation or
	// in a defer.
	job, err := NewJob(ctx, containerCfg, hostCfg)
	return port, job.Stop, job.Errors, err
}

type Job struct {
	Errors       chan error
	StopTimeout  time.Duration
	Stopped      bool
	dockerClient client.CommonAPIClient
	response     types.HijackedResponse
	containerID  string
	sync.Mutex
}

// NewJob runs the defined container.
// Context passed is for initial setup request.  To stop the running job,
// the caller should await context cancelation and call .Stop explicitly, or
// do so in a defer statement.
func NewJob(ctx context.Context, containerCfg container.Config, hostCfg container.HostConfig) (j *Job, err error) {
	j = &Job{
		Errors:      make(chan error, 10),
		StopTimeout: DefaultStopTimeout,
	}

	// Docker Client
	j.dockerClient, _, err = NewClient(client.DefaultDockerHost)
	if err != nil {
		err = errors.Wrap(err, "failed to create Docker API client")
		return
	}

	// Container
	t, err := j.dockerClient.ContainerCreate(ctx, &containerCfg, &hostCfg, nil, nil, "")
	if err != nil {
		err = errors.Wrap(err, "runner unable to create container")
		return
	}
	j.containerID = t.ID

	// Attach to container's stdio
	j.response, err = j.dockerClient.ContainerAttach(ctx, j.containerID,
		types.ContainerAttachOptions{Stdout: true, Stderr: true, Stdin: false, Stream: true})
	if err != nil {
		err = errors.Wrap(err, "runner unable to attach to container's stdio")
		return
	}
	copyErrCh := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(os.Stdout, os.Stderr, j.response.Reader)
		copyErrCh <- err
	}()

	// Channels for receiving body and errors from the container
	bodyCh, waitErrCh := j.dockerClient.ContainerWait(ctx, t.ID, container.WaitConditionNextExit)
	go func() {
		for {
			select {
			case err = <-copyErrCh:
				j.Errors <- err
			case err = <-waitErrCh:
				j.Errors <- err
			case body := <-bodyCh:
				if body.StatusCode != 0 {
					err = fmt.Errorf("container exited with code %v", body.StatusCode)
				}
				j.Errors <- err
			}
		}
	}()

	// Start
	if err = j.dockerClient.ContainerStart(ctx, j.containerID, types.ContainerStartOptions{}); err != nil {
		err = errors.Wrap(err, "runner unable to start container")
		return
	}

	return
}

// Stop the running job, cleaning up.  Errors encountered are printed to stderr,
// with the last of which being returned up the stack.  Nil error indicates no
// errors encoutered stopping.
func (j *Job) Stop() (err error) {
	j.Lock()
	defer j.Unlock()
	if j.Stopped {
		fmt.Fprintf(os.Stderr, "job for container %v already stopped\n", j.containerID)
		return
	}

	err = j.dockerClient.ContainerStop(context.Background(), j.containerID, &j.StopTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error stopping container %v: %v\n", j.containerID, err)
	}
	err = j.dockerClient.ContainerRemove(
		context.Background(), j.containerID, types.ContainerRemoveOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing container %v: %v\n", j.containerID, err)
	}
	err = j.response.Conn.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing connection to container: %v\n", err)
	}
	err = j.dockerClient.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing daemon cliet: %v\n", err)
	}
	j.Stopped = true
	return
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

func newContainerConfig(f fn.Function, _ string, verbose bool) (c container.Config, err error) {
	envs, err := newEnvironmentVariables(f, verbose)
	if err != nil {
		return
	}
	// httpPort := nat.Port(fmt.Sprintf("%v/tcp", port))
	httpPort := nat.Port("8080/tcp")
	return container.Config{
		Image:        f.Image,
		Env:          envs,
		Tty:          false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  false,
		ExposedPorts: map[nat.Port]struct{}{httpPort: {}},
	}, nil
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

func newEnvironmentVariables(f fn.Function, verbose bool) ([]string, error) {
	// TODO: this has code-smell.  It may not be ideal to have fn.Function
	// represent Envs as pointers, as this causes the clearly odd situation of
	// needing to check if an env defined in f is just nil pointers: an invalid
	// data structure.
	envs := []string{}
	for _, env := range f.Envs {
		if env.Name != nil && env.Value != nil {
			value, set, err := processEnvValue(*env.Value)
			if err != nil {
				return envs, err
			}
			if set {
				envs = append(envs, *env.Name+"="+value)
			}
		}
	}
	if verbose {
		envs = append(envs, "VERBOSE=true")
	}
	return envs, nil
}

// run command supports only ENV values in form:
// FOO=bar or FOO={{ env:LOCAL_VALUE }}
var evRegex = regexp.MustCompile(`^{{\s*(\w+)\s*:(\w+)\s*}}$`)

const (
	ctxIdx = 1
	valIdx = 2
)

// processEnvValue returns only value for ENV variable, that is defined in form FOO=bar or FOO={{ env:LOCAL_VALUE }}
// if the value is correct, it is returned and the second return parameter is set to `true`
// otherwise it is set to `false`
// if the specified value is correct, but the required local variable is not set, error is returned as well
func processEnvValue(val string) (string, bool, error) {
	if strings.HasPrefix(val, "{{") {
		match := evRegex.FindStringSubmatch(val)
		if len(match) > valIdx && match[ctxIdx] == "env" {
			if v, ok := os.LookupEnv(match[valIdx]); ok {
				return v, true, nil
			} else {
				return "", false, fmt.Errorf("required local environment variable %q is not set", match[valIdx])
			}
		} else {
			return "", false, nil
		}
	}
	return val, true, nil
}
