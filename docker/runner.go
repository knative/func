package docker

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"

	fn "knative.dev/kn-plugin-func"
)

// Runner of functions using the docker command.
type Runner struct {
	// Verbose logging flag.
	Verbose bool
}

// NewRunner creates an instance of a docker-backed runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run the function.  Errors running are returned along with port.  Runtime
// errors are setnt to the passed error channel.  Stops when the context is
// canceled.
func (n *Runner) Run(ctx context.Context, f fn.Function, errCh chan error) (port int, err error) {
	// Ensure image is built before running
	if f.Image == "" {
		err = errors.New("Function has no associate image. Has it been built?")
		return
	}

	// Port chosen based on OS availability
	port = choosePort()

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

	// Client
	c, _, err := NewClient(client.DefaultDockerHost)
	if err != nil {
		err = errors.Wrap(err, "failed to create Docker API client")
		return
	}
	defer c.Close()

	// Create Container t using client c
	t, err := c.ContainerCreate(ctx, &containerCfg, &hostCfg, nil, nil, "")
	if err != nil {
		err = errors.Wrap(err, "runner unable to create container")
		return
	}

	// Start a routine which when signaled will clean up the container.
	// Set to accept more than enough signals such that the signalsers do
	// not block, but only the first signal matters.
	removeContainerCh := make(chan bool, 10)
	go func() {
		<-removeContainerCh
		if err := c.ContainerRemove(context.Background(), t.ID, types.ContainerRemoveOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "unable remove container '%v': %v", t.ID, err)
		}
	}()

	// Attach the container's stdio to the current process
	resp, err := c.ContainerAttach(ctx, t.ID, types.ContainerAttachOptions{
		Stdout: true, Stderr: true, Stdin: false, Stream: true})
	if err != nil {
		err = errors.Wrap(err, "runner unable to attach to container's stdio")
		removeContainerCh <- true
		return
	}
	defer resp.Close()

	// Start a routine waiting for stiod copy errors
	copyErrCh := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(os.Stdout, os.Stderr, resp.Reader)
		copyErrCh <- err
	}()

	// Start routine waiting for the container exits
	bodyCh, waitErrCh := c.ContainerWait(ctx, t.ID, container.WaitConditionNextExit)

	// Start the container
	if err = c.ContainerStart(ctx, t.ID, types.ContainerStartOptions{}); err != nil {
		err = errors.Wrap(err, "runner unable to start container")
		removeContainerCh <- true
		return
	}

	// Start a routine waiting for the signal to attempt stopping the container
	// Also cascades through to signaling the removal of the container after stop
	stopAndRemoveContainerCh := make(chan bool, 10)
	go func() {
		<-stopAndRemoveContainerCh
		timeout := 10 * time.Second
		err = c.ContainerStop(context.Background(), t.ID, &timeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to stop container: %v", err)
		}
		removeContainerCh <- true
	}()

	// Go wait for errors or cancel
	go func() {
		select {
		case err = <-copyErrCh:
			errCh <- err
		case err = <-waitErrCh:
			errCh <- err
		case <-ctx.Done():
			errCh <- ctx.Err()
		case body := <-bodyCh:
			if body.StatusCode != 0 {
				err = fmt.Errorf("container exited with code %v", body.StatusCode)
			}
			errCh <- err
		}
		stopAndRemoveContainerCh <- true
	}()

	return port, nil
}

// Run the function at path
func (n *Runner) OldRun(ctx context.Context, f fn.Function) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cli, _, err := NewClient(client.DefaultDockerHost)
	if err != nil {
		return errors.Wrap(err, "failed to create docker api client")
	}
	defer cli.Close()

	if f.Image == "" {
		return errors.New("Function has no associated Image. Has it been built? Using the --build flag will build the image if it hasn't been built yet")
	}

	envs := []string{}
	for _, env := range f.Envs {
		if env.Name != nil && env.Value != nil {
			value, set, err := processEnvValue(*env.Value)
			if err != nil {
				return err
			}
			if set {
				envs = append(envs, *env.Name+"="+value)
			}
		}
	}
	if n.Verbose {
		envs = append(envs, "VERBOSE=true")
	}

	httpPort := nat.Port("8080/tcp")
	ports := map[nat.Port][]nat.PortBinding{
		httpPort: {
			nat.PortBinding{
				HostPort: "8080",
				HostIP:   "127.0.0.1",
			},
		},
	}

	conf := &container.Config{
		Env:          envs,
		Tty:          false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  false,
		Image:        f.Image,
		ExposedPorts: map[nat.Port]struct{}{httpPort: {}},
	}

	hostConf := &container.HostConfig{
		PortBindings: ports,
	}

	cont, err := cli.ContainerCreate(ctx, conf, hostConf, nil, nil, "")
	if err != nil {
		return errors.Wrap(err, "failed to create container")
	}
	defer func() {
		err := cli.ContainerRemove(context.Background(), cont.ID, types.ContainerRemoveOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove container: %v", err)
		}
	}()

	attachOptions := types.ContainerAttachOptions{
		Stdout: true,
		Stderr: true,
		Stdin:  false,
		Stream: true,
	}

	resp, err := cli.ContainerAttach(ctx, cont.ID, attachOptions)
	if err != nil {
		return errors.Wrap(err, "failed to attach container")
	}
	defer resp.Close()

	copyErrChan := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(os.Stdout, os.Stderr, resp.Reader)
		copyErrChan <- err
	}()

	waitBodyChan, waitErrChan := cli.ContainerWait(ctx, cont.ID, container.WaitConditionNextExit)

	err = cli.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to start container")
	}
	defer func() {
		t := time.Second * 10
		err := cli.ContainerStop(context.Background(), cont.ID, &t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop container: %v", err)
		}
	}()

	select {
	case body := <-waitBodyChan:
		if body.StatusCode != 0 {
			return fmt.Errorf("failed with status code: %d", body.StatusCode)
		}
	case err := <-waitErrChan:
		return err
	case err := <-copyErrChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// choosePort returns an unused port
// Note this is not fool-proof becase of a race with any other processes
// looking for a port at the same time.
// Note that TCP is presumed.
func choosePort() int {
	// TODO: implement me
	return 8080
}

func newContainerConfig(f fn.Function, port int, verbose bool) (c container.Config, err error) {
	envs, err := newEnvironmentVariables(f, verbose)
	if err != nil {
		return
	}
	httpPort := nat.Port(fmt.Sprintf("%v/tcp", port))
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

func newHostConfig(port int) (c container.HostConfig, err error) {
	httpPort := nat.Port(fmt.Sprintf("%v/tcp", port))
	ports := map[nat.Port][]nat.PortBinding{
		httpPort: {
			nat.PortBinding{
				HostPort: fmt.Sprintf("%v", port),
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
