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
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"

	"github.com/docker/docker/client"

	fn "github.com/boson-project/func"
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

// Run the function at path
func (n *Runner) Run(ctx context.Context, f fn.Function) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return errors.Wrap(err, "failed to create docker api client")
	}

	if f.Image == "" {
		return errors.New("Function has no associated Image. Has it been built?")
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

// run command supports only ENV values in from FOO=bar or FOO={{ env:LOCAL_VALUE }}
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
