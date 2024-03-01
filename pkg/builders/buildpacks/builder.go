package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/project/types"
	"github.com/docker/docker/client"
	"github.com/heroku/color"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
)

// DefaultName when no WithName option is provided to NewBuilder
const DefaultName = builders.Pack

var DefaultBaseBuilder = "ghcr.io/knative/builder-jammy-base:latest"
var DefaultTinyBuilder = "ghcr.io/knative/builder-jammy-tiny:latest"

var (
	DefaultBuilderImages = map[string]string{
		"node":       DefaultBaseBuilder,
		"nodejs":     DefaultBaseBuilder,
		"typescript": DefaultBaseBuilder,
		"go":         DefaultTinyBuilder,
		"python":     DefaultBaseBuilder,
		"quarkus":    DefaultTinyBuilder,
		"rust":       DefaultBaseBuilder,
		"springboot": DefaultBaseBuilder,
	}

	// Ensure that all entries in this list are terminated with a trailing "/"
	// See GHSA-5336-2g3f-9g3m for details
	trustedBuilderImagePrefixes = []string{
		"quay.io/boson/",
		"gcr.io/paketo-buildpacks/",
		"docker.io/paketobuildpacks/",
		"gcr.io/buildpacks/",
		"ghcr.io/knative/",
	}

	defaultBuildpacks = map[string][]string{}
)

// Builder will build Function using Pack.
type Builder struct {
	name    string
	verbose bool
	// in non-verbose mode contains std[err,out], so it can be printed on error
	outBuff       bytes.Buffer
	logger        logging.Logger
	impl          Impl
	withTimestamp bool
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

func WithTimestamp(v bool) Option {
	return func(b *Builder) {
		b.withTimestamp = v
	}
}

var DefaultLifecycleImage = "docker.io/buildpacksio/lifecycle:553c041"

// Build the Function at path.
func (b *Builder) Build(ctx context.Context, f fn.Function, platforms []fn.Platform) (err error) {
	if len(platforms) != 0 {
		return errors.New("the pack builder does not support specifying target platforms directly.")
	}

	// Builder image from the function if defined, default otherwise.
	image, err := BuilderImage(f, b.name)
	if err != nil {
		return
	}

	buildpacks := f.Build.Buildpacks
	if len(buildpacks) == 0 {
		buildpacks = defaultBuildpacks[f.Runtime]
	}

	// Reading .funcignore file
	var excludes []string
	filePath := filepath.Join(f.Root, ".funcignore")
	file, err := os.Open(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("\nfailed to open file: %s", err)
		}
	} else {
		defer file.Close()
		buf := new(bytes.Buffer)
		_, err := io.Copy(buf, file)
		if err != nil {
			return fmt.Errorf("\nfailed to read file: %s", err)
		}
		excludes = strings.Split(buf.String(), "\n")
	}
	// Pack build options
	opts := pack.BuildOptions{
		AppPath:        f.Root,
		Image:          f.Build.Image,
		LifecycleImage: DefaultLifecycleImage,
		Builder:        image,
		Buildpacks:     buildpacks,
		ProjectDescriptor: types.Descriptor{
			Build: types.Build{
				Exclude: excludes,
			},
		},
		ContainerConfig: struct {
			Network string
			Volumes []string
		}{Network: "", Volumes: nil},
	}
	if b.withTimestamp {
		now := time.Now()
		opts.CreationTime = &now
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

		if ok, _ := isPodmanV43(ctx, cli); ok {
			return fmt.Errorf("podman 4.3 is not supported, use podman 4.2 or 4.4")
		}

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

func isPodmanV43(ctx context.Context, cli client.CommonAPIClient) (b bool, err error) {
	version, err := cli.ServerVersion(ctx)
	if err != nil {
		return
	}

	for _, component := range version.Components {
		if component.Name == "Podman Engine" {
			v := semver.MustParse(version.Version)
			if v.Major() == 4 && v.Minor() == 3 {
				return true, nil
			}
			break
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
