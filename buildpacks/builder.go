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

// DefaultBuilderImages for Pack builders indexed by Runtime Language
var DefaultBuilderImages = map[string]string{
	"node":       "gcr.io/paketo-buildpacks/builder:base",
	"typescript": "gcr.io/paketo-buildpacks/builder:base",
	"go":         "gcr.io/paketo-buildpacks/builder:base",
	"python":     "gcr.io/paketo-buildpacks/builder:base",
	"quarkus":    "gcr.io/paketo-buildpacks/builder:base",
	"rust":       "gcr.io/paketo-buildpacks/builder:base",
	"springboot": "gcr.io/paketo-buildpacks/builder:base",
}

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

	// Use the builder found in the Function configuration file if it exists,
	// or a default for the language if not provided
	packBuilder := BuilderImage(f)
	if packBuilder == "" {
		return fmt.Errorf("builder image not found for function of language '%v'", f.Runtime)
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

// Builder Image for a Function being built using Buildpack.
//
// A value defined on the Function itself takes precidence.  If not defined,
// the default builder image for the Function's language runtime is used.
// An inability to determine a builder image (such as an unknown language),
// will return empty string.
//
// Exported for use by Tekton in-cluster builds which do not have access to this
// library at this time, and can therefore not instantiate and invoke this
// package's buildpacks.Builder.Build.  Instead, they must transmit information
// to the cluster using a Pipeline definition.
func BuilderImage(f fn.Function) (builder string) {
	// NOTE this will be updated when func.yaml is expanded to support
	// differing builder images for different build strategies (buildpac vs s2i)
	if f.Builder != "" {
		return f.Builder
	}
	builder = DefaultBuilderImages[f.Runtime]
	return
}
