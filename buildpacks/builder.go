package buildpacks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/docker/docker/client"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/docker"
)

var (
	DefaultBuilderImages = map[string]string{
		"node":       "gcr.io/paketo-buildpacks/builder:base",
		"typescript": "gcr.io/paketo-buildpacks/builder:base",
		"go":         "gcr.io/paketo-buildpacks/builder:base",
		"python":     "gcr.io/paketo-buildpacks/builder:base",
		"quarkus":    "gcr.io/paketo-buildpacks/builder:base",
		"rust":       "gcr.io/paketo-buildpacks/builder:base",
		"springboot": "gcr.io/paketo-buildpacks/builder:base",
	}

	trustedBuilderImagePrefixes = []string{
		"quay.io/boson",
		"gcr.io/paketo-buildpacks",
		"docker.io/paketobuildpacks",
	}

	v330 = semver.MustParse("v3.3.0") // for checking podman version
)

// Builder will build Function using Pack.
type Builder struct {
	verbose bool
	logger  io.Writer
	impl    Impl
}

// Impl allows for the underlying implementation to be mocked for tests.
type Impl interface {
	Build(context.Context, pack.BuildOptions) error
}

// NewBuilder instantiates a Buildpack-based Builder
func NewBuilder(options ...Option) *Builder {
	b := &Builder{}
	for _, o := range options {
		o(b)
	}

	// Stream logs to stdout or buffer only for display on error.
	if b.verbose {
		b.logger = stdoutWrapper{os.Stdout}
	} else {
		b.logger = &bytes.Buffer{}
	}

	return b
}

type Option func(*Builder)

func WithVerbose(v bool) Option {
	return func(b *Builder) {
		b.verbose = v
	}
}

func WithImpl(i Impl) Option {
	return func(b *Builder) {
		b.impl = i
	}
}

// Build the Function at path.
func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	// Builder image defined on the Function if set, or from the default map.
	image, err := BuilderImage(f)
	if err != nil {
		return
	}

	// Pack build options
	opts := pack.BuildOptions{
		AppPath:        f.Root,
		Image:          f.Image,
		LifecycleImage: "quay.io/boson/lifecycle:0.13.2",
		Builder:        image,
		Buildpacks:     f.Buildpacks,
		ContainerConfig: struct {
			Network string
			Volumes []string
		}{Network: "", Volumes: nil},
	}
	if opts.Env, err = fn.Interpolate(f.BuildEnvs); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		opts.ContainerConfig.Network = "host"
	}

	var impl = b.impl
	// Instantate the pack build client implementation
	// (and update build opts as necessary)
	if impl == nil {
		var (
			cli        client.CommonAPIClient
			dockerHost string
		)

		cli, dockerHost, err = docker.NewClient(client.DefaultDockerHost)
		if err != nil {
			return fmt.Errorf("cannot craete docker client: %w", err)
		}
		defer cli.Close()

		if impl, err = newImpl(ctx, cli, dockerHost, &opts, b.logger); err != nil {
			return fmt.Errorf("cannot create pack client: %w", err)
		}
	}

	// Perform the build
	if err = impl.Build(ctx, opts); err != nil {
		if ctx.Err() != nil {
			return // SIGINT
		} else if !b.verbose {
			err = fmt.Errorf("failed to build the function (output: %q): %w", b.logger.(*bytes.Buffer).String(), err)
		}
	}
	return
}

// newImpl returns an instance of the builder implementatoin.  Note that this
// also mutates the provided options' DockerHost and TrustBuilder.
func newImpl(ctx context.Context, cli client.CommonAPIClient, dockerHost string, opts *pack.BuildOptions, logger io.Writer) (impl Impl, err error) {
	opts.DockerHost = dockerHost

	daemonIsPodmanPreV330, err := podmanPreV330(ctx, cli)
	if err != nil {
		return
	}

	opts.TrustBuilder = func(_ string) bool {
		if daemonIsPodmanPreV330 {
			return false
		}
		for _, v := range trustedBuilderImagePrefixes {
			if strings.HasPrefix(opts.Builder, v) {
				return true
			}
		}
		return false
	}

	// Client with a logger which is enabled if in Verbose mode and a dockerClient that supports SSH docker daemon connection.
	return pack.NewClient(pack.WithLogger(logging.NewSimpleLogger(logger)), pack.WithDockerClient(cli))
}

// Builder Image
//
// A value defined on the Function itself takes precidence.  If not defined,
// the default builder image for the Function's language runtime is used.
// An inability to determine a builder image (such as an unknown language),
// will return empty string. Errors are returned if either the runtime is not
// populated or an inability to locate a default.
//
// Exported for use by Tekton in-cluster builds which do not have access to this
// library at this time, and can therefore not instantiate and invoke this
// package's buildpacks.Builder.Build.  Instead, they must transmit information
// to the cluster using a Pipeline definition.
func BuilderImage(f fn.Function) (string, error) {
	if f.Runtime == "" {
		return "", ErrRuntimeRequired{}
	}

	v, ok := f.BuilderImages["pack"]
	if ok {
		return v, nil
	}

	v, ok = DefaultBuilderImages[f.Runtime]
	if ok {
		return v, nil
	}

	return "", ErrRuntimeNotSupported{f.Runtime}
}

// podmanPreV330 returns if the daemon is podman pre v330 or errors trying.
func podmanPreV330(ctx context.Context, cli client.CommonAPIClient) (b bool, err error) {
	version, err := cli.ServerVersion(ctx)
	if err != nil {
		return
	}

	for _, component := range version.Components {
		if component.Name == "Podman Engine" {
			v := semver.MustParse(version.Version)
			if v.Compare(v330) < 0 {
				return true, nil
			}
			break
		}
	}
	return
}

// stdoutWrapper is a hack that makes stdout non-closeable
type stdoutWrapper struct {
	impl io.Writer
}

func (s stdoutWrapper) Write(p []byte) (n int, err error) {
	return s.impl.Write(p)
}

// Errors

type ErrRuntimeRequired struct{}

func (e ErrRuntimeRequired) Error() string {
	return "Pack requires the Function define a language runtime"
}

type ErrRuntimeNotSupported struct {
	Runtime string
}

func (e ErrRuntimeNotSupported) Error() string {
	return fmt.Sprintf("Pack builder has no default builder image for the '%v' language runtime.  Please provide one.", e.Runtime)
}
