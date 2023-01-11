package buildpacks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/docker/docker/client"
	"github.com/heroku/color"

	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/docker"
)

// DefaultName when no WithName option is provided to NewBuilder
const DefaultName = builders.Pack

var (
	DefaultBuilderImages = map[string]string{
		"node":       "gcr.io/paketo-buildpacks/builder:base",
		"nodejs":     "gcr.io/paketo-buildpacks/builder:base",
		"typescript": "gcr.io/paketo-buildpacks/builder:base",
		"go":         "gcr.io/paketo-buildpacks/builder:base",
		"python":     "gcr.io/paketo-buildpacks/builder:base",
		"quarkus":    "gcr.io/paketo-buildpacks/builder:base",
		"rust":       "gcr.io/paketo-buildpacks/builder:base",
		"springboot": "gcr.io/paketo-buildpacks/builder:base",
	}

	// Ensure that all entries in this list are terminated with a trailing "/"
	// See GHSA-5336-2g3f-9g3m for details
	trustedBuilderImagePrefixes = []string{
		"quay.io/boson/",
		"gcr.io/paketo-buildpacks/",
		"docker.io/paketobuildpacks/",
		"ghcr.io/vmware-tanzu/function-buildpacks-for-knative/",
		"gcr.io/buildpacks/",
	}
)

// Builder will build Function using Pack.
type Builder struct {
	name    string
	verbose bool
	// in non-verbose mode contains std[err,out], so it can be printed on error
	outBuff bytes.Buffer
	logger  logging.Logger
	impl    Impl
}

// Impl allows for the underlying implementation to be mocked for tests.
type Impl interface {
	Build(context.Context, pack.BuildOptions) error
}

// NewBuilder instantiates a Buildpack-based Builder
func NewBuilder(options ...Option) *Builder {
	b := &Builder{name: DefaultName}
	for _, o := range options {
		o(b)
	}

	// Stream logs to stdout or buffer only for display on error.
	if b.verbose {
		b.logger = logging.NewLogWithWriters(color.Stdout(), color.Stderr(), logging.WithVerbose())
	} else {
		b.logger = logging.NewSimpleLogger(&b.outBuff)
	}

	return b
}

type Option func(*Builder)

func WithName(n string) Option {
	return func(b *Builder) {
		b.name = n
	}
}

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

var DefaultLifecycleImage = "quay.io/boson/lifecycle@sha256:1c22b303836cb31330c4239485c62b9bfb418a6d83cd2b3a842506540300e485"

// Build the Function at path.
func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	// Builder image from the function if defined, default otherwise.
	image, err := BuilderImage(f, b.name)
	if err != nil {
		return
	}

	// Pack build options
	opts := pack.BuildOptions{
		AppPath:        f.Root,
		Image:          f.Image,
		LifecycleImage: DefaultLifecycleImage,
		Builder:        image,
		Buildpacks:     f.Build.Buildpacks,
		ContainerConfig: struct {
			Network string
			Volumes []string
		}{Network: "", Volumes: nil},
	}
	if opts.Env, err = fn.Interpolate(f.Build.BuildEnvs); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		opts.ContainerConfig.Network = "host"
	}

	// only trust our known builders
	opts.TrustBuilder = TrustBuilder

	var impl = b.impl
	// Instantiate the pack build client implementation
	// (and update build opts as necessary)
	if impl == nil {
		var (
			cli        client.CommonAPIClient
			dockerHost string
		)

		cli, dockerHost, err = docker.NewClient(client.DefaultDockerHost)
		if err != nil {
			return fmt.Errorf("cannot create docker client: %w", err)
		}
		defer cli.Close()
		opts.DockerHost = dockerHost

		// Client with a logger which is enabled if in Verbose mode and a dockerClient that supports SSH docker daemon connection.
		if impl, err = pack.NewClient(pack.WithLogger(b.logger), pack.WithDockerClient(cli)); err != nil {
			return fmt.Errorf("cannot create pack client: %w", err)
		}
	}

	// Perform the build
	if err = impl.Build(ctx, opts); err != nil {
		if ctx.Err() != nil {
			return // SIGINT
		} else if !b.verbose {
			err = fmt.Errorf("failed to build the function: %w", err)
			fmt.Fprintln(color.Stderr(), "")
			_, _ = io.Copy(color.Stderr(), &b.outBuff)
			fmt.Fprintln(color.Stderr(), "")
		}
	}
	return
}

// TrustBuilder determines whether the builder image should be trusted
// based on a set of trusted builder image registry prefixes.
func TrustBuilder(b string) bool {
	for _, v := range trustedBuilderImagePrefixes {
		// Ensure that all entries in this list are terminated with a trailing "/"
		if !strings.HasSuffix(v, "/") {
			v = v + "/"
		}
		if strings.HasPrefix(b, v) {
			return true
		}
	}
	return false
}

// Builder Image chooses the correct builder image or defaults.
func BuilderImage(f fn.Function, builderName string) (string, error) {
	return builders.Image(f, builderName, DefaultBuilderImages)
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
