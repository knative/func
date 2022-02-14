package s2i

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	dockerClient "github.com/docker/docker/client"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/validation"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/scm/git"

	fn "knative.dev/kn-plugin-func"
	docker "knative.dev/kn-plugin-func/docker"
)

// DefaultBuilderImages for s2i builders indexed by Runtime Language
var DefaultBuilderImages = map[string]string{
	"node": "registry.access.redhat.com/ubi8/nodejs-16", // TODO: finalize choice and include version
}

type Builder struct {
	Verbose bool
}

func NewBuilder() *Builder {
	return &Builder{}
}

// defaultBuilderImage for the given function based on its runtime, or an
// error if no default is defined for the given runtime.
func defaultBuilderImage(f fn.Function) (string, error) {
	v, ok := DefaultBuilderImages[f.Runtime]
	if !ok {
		return "", fmt.Errorf("S2I builder has no default builder image specified for the '%v' language runtime.  Please provide one.", f.Runtime)
	}
	return v, nil
}

func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	// Ensure the Function has a builder specified
	if f.Builder == "" {
		f.Builder, err = defaultBuilderImage(f)
		if err != nil {
			return
		}
	}

	client, endpoint, err := docker.NewClient(dockerClient.DefaultDockerHost)
	if err != nil {
		return err
	}
	if endpoint == "" {
		endpoint = dockerClient.DefaultDockerHost // TODO: Should not need to do this.
	}
	defer client.Close()

	cfg := &api.Config{}
	cfg.Tag = f.Image
	cfg.Source = &git.URL{URL: url.URL{Path: f.Root}, Type: git.URLTypeLocal}
	cfg.BuilderImage = f.Builder
	cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
	cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
	cfg.RuntimeImagePullPolicy = api.DefaultRuntimeImagePullPolicy
	cfg.DockerConfig = &api.DockerConfig{
		Endpoint: endpoint,
	}
	cfg.Quiet = !b.Verbose

	if errs := validation.ValidateConfig(cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
		}
		return errors.New("Unable to build via the s2i builder.")
	}

	// Environment Variables
	// TODO: I still don't understand why f.Envs has nils.
	for _, env := range f.Envs {
		if env.Name != nil && env.Value != nil {
			cfg.Environment = append(cfg.Environment,
				api.EnvironmentSpec{Name: *env.Name, Value: *env.Value})
		}
	}

	// Volumes
	// TODO: I still don't understand why f.Labels has nils.
	cfg.Labels = make(map[string]string)
	for _, label := range f.Labels {
		if label.Key != nil && label.Value != nil {
			cfg.Labels[*label.Key] = *label.Value
		}
	}

	// Create a builder impl from the docker client and config; build and
	// print any resulting messages to stdout.
	builder, _, err := strategies.Strategy(client, cfg, build.Overrides{})
	if err != nil {
		return
	}
	result, err := builder.Build(cfg)
	if err != nil {
		return
	}
	if b.Verbose {
		for _, message := range result.Messages {
			fmt.Println(message)
		}
	}
	return
}

// processEnvValue
//
// TODO: The following environment variable processing code/regexes were
// copied varbatim from the buildpacks package.  It should be refactored to be
// modular and extracted for use in both places.

var buildEnvRegex = regexp.MustCompile(`^{{\s*(\w+)\s*:(\w+)\s*}}$`)

const (
	ctxIdx = 1
	valIdx = 2
)

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
