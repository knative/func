package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

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
		c    DockerClient // Docker client
		id   string       // ID of running container
		conn net.Conn     // Connection to container's stdio

		// Channels for gathering runtime errors from the container instance
		copyErrCh = make(chan error, 10)

		// Combined runtime error channel for sending all errors to caller
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
	if c, _, err = NewClient(client.DefaultDockerHost); err != nil {
		return job, errors.Wrap(err, "failed to create Docker API client")
	}
	if id, err = newContainer(ctx, c, f, host, port, n.verbose, n.errOut); err != nil {
		return job, errors.Wrap(err, "runner unable to create container")
	}
	if conn, err = copyStdio(ctx, c, id, copyErrCh, n.out, n.errOut); err != nil {
		return
	}

	// Wait for errors premature exits
	contWaitResult := c.ContainerWait(ctx, id, client.ContainerWaitOptions{
		Condition: container.WaitConditionNextExit,
	})
	go func() {
		for {
			select {
			case err = <-copyErrCh:
				runtimeErrCh <- err
			case body := <-contWaitResult.Result:
				// NOTE: currently an exit is not expected and thus a return, for any
				// reason, is considered an error even when the exit code is 0.
				// Functions are expected to be long-running processes that do not exit
				// of their own accord when run locally.  Should this expectation
				// change in the future, this channel-based wait may need to be
				// expanded to accept the case of a voluntary, successful exit.
				runtimeErrCh <- fmt.Errorf("exited code %v", body.StatusCode)
			case err = <-contWaitResult.Error:
				runtimeErrCh <- err
			}
		}
	}()

	if _, err = c.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return job, errors.Wrap(err, "runner unable to start container")
	}

	readyCh := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(startTimeout)
		for {
			if time.Now().After(deadline) {
				readyCh <- fmt.Errorf("container did not become ready in %v", startTimeout)
				return
			}
			if err = dial(host, port, 500*time.Millisecond); err == nil {
				readyCh <- nil
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case err = <-readyCh:
		if err != nil {
			return job, err
		}
	case <-ctx.Done():
		return job, ctx.Err()
	}

	// Stopper
	stop := func() error {
		var (
			timeout = DefaultStopTimeout
			ctx     = context.Background()
		)
		timeoutSecs := int(timeout.Seconds())
		ctrStopOpts := client.ContainerStopOptions{
			Timeout: &timeoutSecs,
		}
		if _, err = c.ContainerStop(ctx, id, ctrStopOpts); err != nil {
			return fmt.Errorf("error stopping container %v: %v", id, err)
		}
		if _, err = c.ContainerRemove(ctx, id, client.ContainerRemoveOptions{}); err != nil {
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

func newContainer(ctx context.Context, c DockerClient, f fn.Function, host, port string, verbose bool, errOut io.Writer) (id string, err error) {
	var (
		containerCfg container.Config
		hostCfg      container.HostConfig
	)
	if containerCfg, err = newContainerConfig(f, port, verbose); err != nil {
		return
	}
	if hostCfg, err = newHostConfig(host, port, f, errOut); err != nil {
		return
	}
	t, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &containerCfg,
		HostConfig: &hostCfg,
	})
	if err != nil {
		return
	}
	return t.ID, nil
}

func newContainerConfig(f fn.Function, _ string, verbose bool) (c container.Config, err error) {
	httpPort := network.MustParsePort("8080/tcp")
	c = container.Config{
		Image:        f.Build.Image,
		Tty:          false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  false,
		ExposedPorts: network.PortSet{httpPort: {}},
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
	if k := f.Run.Kafka; k != nil && k.Brokers != "" && k.Topic != "" && k.ConsumerGroup != "" {
		c.Env = append(c.Env,
			"FUNC_TRANSPORT=kafka",
			"KAFKA_BROKERS="+k.Brokers,
			"KAFKA_TOPIC="+k.Topic,
			"KAFKA_CONSUMER_GROUP="+k.ConsumerGroup,
		)
		if k.SecurityProtocol != "" {
			c.Env = append(c.Env, "KAFKA_SECURITY_PROTOCOL="+k.SecurityProtocol)
		}
		if k.TLS != nil {
			if k.TLS.CACert != "" {
				c.Env = append(c.Env, "KAFKA_TLS_CA_CERT="+k.TLS.CACert)
			}
			if k.TLS.ClientCert != "" {
				c.Env = append(c.Env, "KAFKA_TLS_CLIENT_CERT="+k.TLS.ClientCert)
			}
			if k.TLS.ClientKey != "" {
				c.Env = append(c.Env, "KAFKA_TLS_CLIENT_KEY="+k.TLS.ClientKey)
			}
			if k.TLS.SkipVerify {
				c.Env = append(c.Env, "KAFKA_TLS_SKIP_VERIFY=true")
			} else {
				c.Env = append(c.Env, "KAFKA_TLS_SKIP_VERIFY=false")
			}
		}
		if k.SASL != nil {
			c.Env = append(c.Env, "KAFKA_SASL_MECHANISM="+k.SASL.EffectiveMechanism())
			if k.SASL.User != "" {
				c.Env = append(c.Env, "KAFKA_SASL_USER="+k.SASL.User)
			}
			if k.SASL.Password != "" {
				c.Env = append(c.Env, "KAFKA_SASL_PASSWORD="+k.SASL.Password)
			}
		}
	}

	return
}

func newHostConfig(host, port string, f fn.Function, errOut io.Writer) (c container.HostConfig, err error) {
	httpPort := network.MustParsePort("8080/tcp")
	hostIP, err := netip.ParseAddr(host)
	if err != nil {
		return c, fmt.Errorf("invalid host IP %q: %w", host, err)
	}
	ports := network.PortMap{
		httpPort: {
			{
				HostPort: port,
				HostIP:   hostIP,
			},
		},
	}
	binds, mounts := volumeMounts(f.Root, f.Run.Volumes, errOut)
	return container.HostConfig{PortBindings: ports, Binds: binds, Mounts: mounts}, nil
}

// volumeMounts converts func.yaml volume definitions into Docker host config.
// Secret and ConfigMap binds go on Binds as "src:dst:ro,z" so Docker and Podman
// relabel for SELinux; non-SELinux hosts ignore the z flag. A source or target
// that contains ":" stays a read-only Mount instead: the Linux daemon splits a
// bind spec on colon, so a drive letter or a colon in the path would make
// container create fail, and :z buys nothing on Windows. emptyDir and PVC
// stay on Mounts (tmpfs and named volumes). mount.Mount has no relabel field.
// Volumes that cannot be mapped locally emit a warning on errOut and are
// skipped rather than aborting the run. A nil errOut is treated as io.Discard.
// Creating a secret or configMap directory prints one line: the directory is
// empty until the user adds a file per key.
func volumeMounts(root string, volumes []fn.Volume, errOut io.Writer) (binds []string, mounts []mount.Mount) {
	if errOut == nil {
		errOut = io.Discard
	}
	for _, vol := range volumes {
		if vol.Path == nil {
			fmt.Fprintf(errOut, "warning: skipping volume %s: missing path\n", vol.String())
			continue
		}
		kind, src, isBind := localBind(root, vol)
		fresh := false
		if isBind {
			_, err := os.Stat(src)
			fresh = os.IsNotExist(err)
		}
		m, err := toMount(root, vol)
		if err != nil {
			fmt.Fprintf(errOut, "warning: skipping volume %s: %v\n", vol.String(), err)
			continue
		}
		if fresh {
			fmt.Fprintf(errOut, "created empty local %s dir %s; add files named after the %s's keys to populate it\n", kind, src, kind)
		}
		if m.Type == mount.TypeBind {
			if strings.Contains(m.Source, ":") || strings.Contains(m.Target, ":") {
				fmt.Fprintf(errOut, "warning: %s bound without SELinux relabel (path contains ':')\n", vol.String())
				mounts = append(mounts, m)
				continue
			}
			binds = append(binds, m.Source+":"+m.Target+":ro,z")
			continue
		}
		mounts = append(mounts, m)
	}
	return binds, mounts
}

// localBind is the host path toMount will use for a secret or configMap.
// The kind is the word used in the create-time message.
func localBind(root string, vol fn.Volume) (kind, src string, ok bool) {
	runDir := filepath.Join(root, fn.RunDataDir, "run")
	switch {
	case vol.Secret != nil:
		return "secret", filepath.Join(runDir, "secrets", *vol.Secret), true
	case vol.ConfigMap != nil:
		return "configmap", filepath.Join(runDir, "configmaps", *vol.ConfigMap), true
	default:
		return "", "", false
	}
}

// safeLocalName accepts only a DNS-1123 subdomain, the same rule Kubernetes
// uses for Secret and ConfigMap names. That rejects empty, ".", "..",
// uppercase, separators, and consecutive dots. "." is not just a bad name:
// filepath.Join collapses it to the parent directory and the bind would
// expose every local secret at once.
func safeLocalName(name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("name %q must be a DNS-1123 subdomain: %s", name, strings.Join(errs, "; "))
	}
	return nil
}

// prepareBindDir creates src when it is missing and makes that directory
// readable by a non-root container user. An existing directory keeps the
// mode the user set. Only the leaf matters: the engine bind-mounts src, so
// the container does not walk the host ancestors. chmod after mkdir so a
// tight umask cannot leave the new directory owner-only.
func prepareBindDir(src string) (created bool, err error) {
	if _, err := os.Stat(src); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(src, 0755); err != nil {
		return false, err
	}
	if err := os.Chmod(src, 0755); err != nil {
		return false, err
	}
	return true, nil
}

// toMount maps a single Volume spec to a Docker mount.Mount.
//
// runDir is <root>/.func/run (fn.RunDataDir = ".func", the extra "run" segment
// scopes local state for the run subcommand away from build artifacts).
//
// Mapping rules:
//   - Secret    → read-only bind from <runDir>/secrets/<name> (created 0755 if absent),
//     placed on HostConfig.Binds as src:dst:ro,z
//   - ConfigMap → read-only bind from <runDir>/configmaps/<name> (created 0755 if absent),
//     placed on HostConfig.Binds as src:dst:ro,z
//   - EmptyDir  → tmpfs for both default and Memory mediums. A sizeLimit, when
//     set, becomes TmpfsOptions.SizeBytes. This is not an anonymous volume:
//     those persist across runs and leak storage.
//   - PVC       → named Docker volume keyed by claimName (not a bind under
//     .func/run/pvcs). Shared across functions, matching Kubernetes. ReadOnly
//     is forwarded from the claim.
//
// A newly created directory is 0755, not 0700. Function images run as a
// non-root user, and rootless Podman maps that uid away from the host owner,
// so a 0700 bind is unreadable. An existing directory is left alone. The
// engine bind-mounts the directory itself, so parent modes on the host do
// not matter to the container.
//
// SELinux: secret and ConfigMap binds use HostConfig.Binds with the :z
// relabel suffix, which Docker and Podman honor and non-SELinux hosts
// ignore. emptyDir and PVC stay on Mounts, which has no relabel field.
func toMount(root string, vol fn.Volume) (mount.Mount, error) {
	if vol.Path == nil {
		return mount.Mount{}, fmt.Errorf("missing path")
	}
	target := *vol.Path
	// fn.RunDataDir = ".func"; the "run" subdirectory scopes local run state.
	runDir := filepath.Join(root, fn.RunDataDir, "run")

	if vol.Secret != nil {
		if err := safeLocalName(*vol.Secret); err != nil {
			return mount.Mount{}, fmt.Errorf("secret: %w", err)
		}
		src := filepath.Join(runDir, "secrets", *vol.Secret)
		if _, err := prepareBindDir(src); err != nil {
			return mount.Mount{}, fmt.Errorf("cannot create local secret dir %q: %w", src, err)
		}
		return mount.Mount{Type: mount.TypeBind, Source: src, Target: target, ReadOnly: true}, nil
	}

	if vol.ConfigMap != nil {
		if err := safeLocalName(*vol.ConfigMap); err != nil {
			return mount.Mount{}, fmt.Errorf("configMap: %w", err)
		}
		src := filepath.Join(runDir, "configmaps", *vol.ConfigMap)
		if _, err := prepareBindDir(src); err != nil {
			return mount.Mount{}, fmt.Errorf("cannot create local configmap dir %q: %w", src, err)
		}
		return mount.Mount{Type: mount.TypeBind, Source: src, Target: target, ReadOnly: true}, nil
	}

	if vol.EmptyDir != nil {
		// Both mediums map to tmpfs locally: EmptyDir is ephemeral (pod-lifetime)
		// and anonymous Docker volumes persist across runs, leaking storage.
		m := mount.Mount{Type: mount.TypeTmpfs, Target: target}
		if vol.EmptyDir.SizeLimit != nil && *vol.EmptyDir.SizeLimit != "" {
			q, err := resource.ParseQuantity(*vol.EmptyDir.SizeLimit)
			if err != nil {
				return mount.Mount{}, fmt.Errorf("emptyDir sizeLimit: %w", err)
			}
			m.TmpfsOptions = &mount.TmpfsOptions{SizeBytes: q.Value()}
		}
		return m, nil
	}

	if vol.PersistentVolumeClaim != nil {
		if vol.PersistentVolumeClaim.ClaimName == nil {
			return mount.Mount{}, fmt.Errorf("persistentVolumeClaim missing claimName")
		}
		// Named Docker volume keyed by claimName: multiple functions referencing
		// the same claim share the same local volume, matching Kubernetes semantics.
		return mount.Mount{
			Type:     mount.TypeVolume,
			Source:   *vol.PersistentVolumeClaim.ClaimName,
			Target:   target,
			ReadOnly: vol.PersistentVolumeClaim.ReadOnly,
		}, nil
	}

	return mount.Mount{}, fmt.Errorf("unrecognized volume type")
}

// copy stdin and stdout from the container of the given ID.  Errors encountered
// during copy are communicated via a provided errs channel.
func copyStdio(ctx context.Context, c DockerClient, id string, errs chan error, out, errOut io.Writer) (conn net.Conn, err error) {
	res, err := c.ContainerAttach(ctx, id, client.ContainerAttachOptions{
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
