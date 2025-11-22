package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	"knative.dev/func/pkg/scaffolding"
)

// DefaultName when no WithName option is provided to NewBuilder
const DefaultName = builders.Pack

var DefaultBaseBuilder = "ghcr.io/gauron99/builder-jammy-base:latest"
var DefaultTinyBuilder = "ghcr.io/gauron99/builder-jammy-tiny:latest"

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
		"quay.io/dfridric/",
		"quay.io/boson/",
		"gcr.io/paketo-buildpacks/",
		"docker.io/paketobuildpacks/",
		"gcr.io/buildpacks/",
		"ghcr.io/knative/",
		"docker.io/heroku/",
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

var DefaultLifecycleImage = "docker.io/buildpacksio/lifecycle:3659764"

// Build the Function at path.
func (b *Builder) Build(ctx context.Context, f fn.Function, platforms []fn.Platform) (err error) {
	if len(platforms) != 0 {
		return errors.New("the pack builder does not support specifying target platforms directly")
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
		GroupID:        -1,
		UserID:         -1,
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

	// NOTE: gauron99 - this might be even created into a Client function and
	// ran before the client.Build() all together (in the CLI). There are gonna
	// be commonalitites across the builders for scaffolding with some nuances
	// which could be handled by each "scaffolder" - similarly to builders.
	// scaffold
	if err = scaffold(f); err != nil {
		return
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

	if _, ok := opts.Env["BPE_DEFAULT_LISTEN_ADDRESS"]; !ok {
		opts.Env["BPE_DEFAULT_LISTEN_ADDRESS"] = "[::]:8080"
	}

	// go specific workdir set to where main is
	if f.Runtime == "go" {
		if _, ok := opts.Env["BP_GO_WORKDIR"]; !ok {
			opts.Env["BP_GO_WORKDIR"] = ".func/builds/last"
		}
	}
	var bindings = make([]string, 0, len(f.Build.Mounts))
	for _, m := range f.Build.Mounts {
		bindings = append(bindings, fmt.Sprintf("%s:%s", m.Source, m.Destination))
	}
	opts.ContainerConfig.Volumes = bindings

	// only trust our known builders
	opts.TrustBuilder = TrustBuilder

	var impl = b.impl
	// Instantiate the pack build client implementation
	// (and update build opts as necessary)
	if impl == nil {
		var (
			cli        client.APIClient
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

		if f.Runtime == "python" {
			if fi, _ := os.Lstat(filepath.Join(f.Root, "Procfile")); fi == nil {
				// HACK (of a hack): need to get the right invocation signature
				// the standard scaffolding does this in toSignature() func.
				// we know we have python here.
				invoke := f.Invoke
				if invoke == "" {
					invoke = "http"
				}
				cli = pyScaffoldInjector{cli, invoke}
			}
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
		} else if b.verbose {
			err = fmt.Errorf("failed to build the function: %w", err)
			fmt.Fprintln(color.Stderr(), "")
			_, _ = io.Copy(color.Stderr(), &b.outBuff)
			fmt.Fprintln(color.Stderr(), "")
		}
	}
	return
}

func isPodmanV43(ctx context.Context, cli client.APIClient) (b bool, err error) {
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
	if isLocalhost(b) {
		return true
	}
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

func isLocalhost(img string) bool {
	// Parsing logic is broken for localhost in go-containerregistry.
	// See: https://github.com/google/go-containerregistry/issues/2048
	// So I went for regex.
	localhostRE := regexp.MustCompile(`^(localhost|127\.0\.0\.1|\[::1\])(:\d+)?/.+$`)
	return localhostRE.MatchString(img)
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

// TODO: gauron99 - unify this with other builders; temporary for the go pack
//
// scaffold the project
func scaffold(f fn.Function) error {
	// Scafffolding is currently only supported by the Go runtime
	// Python currently uses an injector instead of this
	if f.Runtime != "go" {
		return nil
	}

	contextDir := filepath.Join(".func", "builds", "last")
	appRoot := filepath.Join(f.Root, contextDir)
	_ = os.RemoveAll(appRoot)

	// The embedded repository contains the scaffolding code itself which glues
	// together the middleware and a function via main
	embeddedRepo, err := fn.NewRepository("", "") // default is the embedded fs
	if err != nil {
		return fmt.Errorf("unable to load the embedded scaffolding. %w", err)
	}

	// Write scaffolding to .func/builds/last
	err = scaffolding.Write(appRoot, f.Root, f.Runtime, f.Invoke, embeddedRepo.FS())
	if err != nil {
		return fmt.Errorf("unable to build due to a scaffold error. %w", err)
	}
	return nil
}
