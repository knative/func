package buildpacks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/docker/docker/client"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/docker"

	"github.com/Masterminds/semver"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

//Builder holds the configuration that will be passed to
//Buildpack builder
type Builder struct {
	verbose bool
}

//NewBuilder builds the new Builder configuration
func NewBuilder(verbose bool) *Builder {
	return &Builder{verbose: verbose}
}

var v330 = semver.MustParse("v3.3.0")

// Build the Function at path.
func (builder *Builder) Build(ctx context.Context, f fn.Function) (err error) {

	// Use the builder found in the Function configuration file
	var packBuilder string
	if f.Builder != "" {
		packBuilder = f.Builder
	} else {
		return fmt.Errorf("No builder image configured")
	}

	// Build options for the pack client.
	var network string
	if runtime.GOOS == "linux" {
		network = "host"
	}

	// log output is either STDOUt or kept in a buffer to be printed on error.
	var logWriter io.Writer
	if builder.verbose {
		// pass stdout as non-closeable writer
		// otherwise pack client would close it which is bad
		logWriter = stdoutWrapper{os.Stdout}
	} else {
		logWriter = &bytes.Buffer{}
	}

	cli, dockerHost, err := docker.NewClient(client.DefaultDockerHost)
	if err != nil {
		return err
	}
	defer cli.Close()

	version, err := cli.ServerVersion(ctx)
	if err != nil {
		return err
	}

	var daemonIsPodmanBeforeV330 bool
	for _, component := range version.Components {
		if component.Name == "Podman Engine" {
			v := semver.MustParse(version.Version)
			if v.Compare(v330) < 0 {
				daemonIsPodmanBeforeV330 = true
			}
			break
		}
	}

	buildEnvs, err := fn.Interpolate(f.BuildEnvs)
	if err != nil {
		return err
	}

	var isTrustedBuilderFunc = func(b string) bool {
		return !daemonIsPodmanBeforeV330 &&
			(strings.HasPrefix(packBuilder, "quay.io/boson") ||
				strings.HasPrefix(packBuilder, "gcr.io/paketo-buildpacks") ||
				strings.HasPrefix(packBuilder, "docker.io/paketobuildpacks"))
	}
	packOpts := pack.BuildOptions{
		AppPath:        f.Root,
		Image:          f.Image,
		LifecycleImage: "quay.io/boson/lifecycle:0.13.2",
		Builder:        packBuilder,
		Env:            buildEnvs,
		Buildpacks:     f.Buildpacks,
		TrustBuilder:   isTrustedBuilderFunc,
		DockerHost:     dockerHost,
		ContainerConfig: struct {
			Network string
			Volumes []string
		}{Network: network, Volumes: nil},
	}

	// Client with a logger which is enabled if in Verbose mode and a dockerClient that supports SSH docker daemon connection.
	packClient, err := pack.NewClient(pack.WithLogger(logging.NewSimpleLogger(logWriter)), pack.WithDockerClient(cli))
	if err != nil {
		return
	}

	// Build based using the given builder.
	if err = packClient.Build(ctx, packOpts); err != nil {
		if ctx.Err() != nil {
			// received SIGINT
			return
		} else if !builder.verbose {
			// If the builder was not showing logs, embed the full logs in the error.
			err = fmt.Errorf("failed to build the function (output: %q): %w", logWriter.(*bytes.Buffer).String(), err)
		}
	}

	return
}

// hack this makes stdout non-closeable
type stdoutWrapper struct {
	impl io.Writer
}

func (s stdoutWrapper) Write(p []byte) (n int, err error) {
	return s.impl.Write(p)
}
