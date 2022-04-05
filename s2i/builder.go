package s2i

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	dockerClient "github.com/docker/docker/client"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/validation"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	s2idocker "github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/scm/git"

	fn "knative.dev/kn-plugin-func"
	docker "knative.dev/kn-plugin-func/docker"
)

var (
	// ErrRuntimeRequired indicates the required value of Function Runtime was not provided
	ErrRuntimeRequired = errors.New("runtime is required to build")

	// ErrRuntimeNotSupported indicates the given runtime is not (yet) supported
	// by this builder.
	ErrRuntimeNotSupported = errors.New("runtime not supported")
)

// DefaultBuilderImages for s2i builders indexed by Runtime Language
var DefaultBuilderImages = map[string]string{
	"node": "registry.access.redhat.com/ubi8/nodejs-16", // TODO: finalize choice and include version
}

// Builder of Functions using the s2i subsystem.
type Builder struct {
	verbose bool
}

// NewBuilder creates a new instance of a Builder with static defaults.
func NewBuilder(verbose bool) *Builder {
	return &Builder{verbose: verbose}
}

func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	// Ensure the Function has a builder specified
	if f.Builder == "" {
		f.Builder, err = defaultBuilderImage(f)
		if err != nil {
			return
		}
	}

	client, _, err := docker.NewClient(dockerClient.DefaultDockerHost)
	if err != nil {
		return err
	}
	defer client.Close()

	// Build Config
	cfg := &api.Config{}
	cfg.Quiet = !b.verbose
	cfg.Tag = f.Image
	cfg.Source = &git.URL{URL: url.URL{Path: f.Root}, Type: git.URLTypeLocal}
	cfg.BuilderImage = f.Builder
	cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
	cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
	cfg.RuntimeImagePullPolicy = api.DefaultRuntimeImagePullPolicy
	cfg.DockerConfig = s2idocker.GetDefaultDockerConfig()
	if errs := validation.ValidateConfig(cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
		}
		return errors.New("Unable to build via the s2i builder.")
	}

	builder, _, err := strategies.Strategy(client, cfg, build.Overrides{})
	if err != nil {
		return
	}

	result, err := builder.Build(cfg)
	if err != nil {
		return
	}

	if b.verbose {
		for _, message := range result.Messages {
			fmt.Println(message)
		}
	}
	return
}

// defaultBuilderImage for the given function based on its runtime, or an
// error if no default is defined for the given runtime.
func defaultBuilderImage(f fn.Function) (string, error) {
	if f.Runtime == "" {
		return "", ErrRuntimeRequired
	}
	v, ok := DefaultBuilderImages[f.Runtime]
	if !ok {
		return "", ErrRuntimeNotSupported
	}
	return v, nil
}
