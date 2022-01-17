package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
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
	Verbose bool
}

//NewBuilder builds the new Builder configuration
func NewBuilder() *Builder {
	return &Builder{}
}

var v330 = semver.MustParse("v3.3.0")

// Build the Function at path.
func (builder *Builder) Build(ctx context.Context, f fn.Function) (err error) {

	// Use the builder found in the Function configuration file
	var packBuilder string
	if f.Builder != "" {
		packBuilder = f.Builder
		pb, ok := f.Builders[packBuilder]
		if ok {
			packBuilder = pb
		}
	} else {
		return errors.New("no buildpack configured for function")
	}

	// Build options for the pack client.
	var network string
	if runtime.GOOS == "linux" {
		network = "host"
	}

	// log output is either STDOUt or kept in a buffer to be printed on error.
	var logWriter io.Writer
	if builder.Verbose {
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

	buildEnvs := make(map[string]string, len(f.BuildEnvs))
	for _, env := range f.BuildEnvs {
		val, set, err := processEnvValue(*env.Value)
		if err != nil {
			return err
		}
		if set {
			buildEnvs[*env.Name] = val
		}
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
		} else if !builder.Verbose {
			// If the builder was not showing logs, embed the full logs in the error.
			err = fmt.Errorf("%v\noutput: %s\n", err, logWriter.(*bytes.Buffer).String())
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

// build command supports only ENV values in from FOO=bar or FOO={{ env:LOCAL_VALUE }}
var buildEnvRegex = regexp.MustCompile(`^{{\s*(\w+)\s*:(\w+)\s*}}$`)

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
		match := buildEnvRegex.FindStringSubmatch(val)
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
