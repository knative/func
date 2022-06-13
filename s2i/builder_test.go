package s2i_test

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types"
	"io"
	"strings"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/s2i"
	. "knative.dev/kn-plugin-func/testing"
)

// Test_ErrRuntimeRequired ensures that a request to build without a runtime
// defined for the Function yields an ErrRuntimeRequired
func Test_ErrRuntimeRequired(t *testing.T) {
	b := s2i.NewBuilder()
	err := b.Build(context.Background(), fn.Function{})

	if !errors.Is(err, s2i.ErrRuntimeRequired) {
		t.Fatal("expected ErrRuntimeRequired not received")
	}
}

// Test_ErrRuntimeNotSupported ensures that a request to build a function whose
// runtime is not yet supported yields an ErrRuntimeNotSupported
func Test_ErrRuntimeNotSupported(t *testing.T) {
	b := s2i.NewBuilder()
	err := b.Build(context.Background(), fn.Function{Runtime: "unsupported"})

	if !errors.Is(err, s2i.ErrRuntimeNotSupported) {
		t.Fatal("expected ErrRuntimeNotSupported not received")
	}
}

// Test_BuilderImageDefault ensures that a Function being built which does not
// define a Builder Image will default.
func Test_ImageDefault(t *testing.T) {
	var (
		i = &mockImpl{}                                              // mock underlying s2i implementation
		c = noopDockerClient{}                                       // mock docker client
		b = s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c)) // Func S2I Builder logic
		f = fn.Function{Runtime: "node"}                             // Function with no builder image set
	)

	// An implementation of the underlying S2I implementation which verifies
	// the config has arrived as expected (correct Functions logic applied)
	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		expected := s2i.DefaultBuilderImages["node"]
		if cfg.BuilderImage != expected {
			t.Fatalf("expected s2i config builder image '%v', got '%v'", expected, cfg.BuilderImage)
		}
		return nil, nil
	}

	// Invoke Build, which runs Function Builder logic before invoking the
	// mock impl above.
	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// Test_BuilderImageConfigurable ensures that the builder will use the builder
// image defined on the given Function if provided.
func Test_BuilderImageConfigurable(t *testing.T) {
	var (
		i = &mockImpl{}                                              // mock underlying s2i implementation
		c = noopDockerClient{}                                       // mock docker client
		b = s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c)) // Func S2I Builder logic
		f = fn.Function{                                             // Function with a builder image set
			Runtime: "node",
			BuilderImages: map[string]string{
				"s2i": "example.com/user/builder-image",
			},
		}
	)

	// An implementation of the underlying S2I implementation which verifies
	// the config has arrived as expected (correct Functions logic applied)
	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		expected := f.BuilderImages["s2i"]
		if cfg.BuilderImage != expected {
			t.Fatalf("expected s2i config builder image for node to be '%v', got '%v'", expected, cfg.BuilderImage)
		}
		return nil, nil
	}

	// Invoke Build, which runs Function Builder logic before invoking the
	// mock impl above.
	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// Test_Verbose ensures that the verbosity flag is propagated to the
// S2I builder implementation.
func Test_BuilderVerbose(t *testing.T) {
	c := noopDockerClient{} // mock docker client
	assert := func(verbose bool) {
		i := &mockImpl{
			BuildFn: func(cfg *api.Config) (r *api.Result, err error) {
				if cfg.Quiet == verbose {
					t.Fatalf("expected s2i quiet mode to be !%v when verbose %v", verbose, verbose)
				}
				return &api.Result{Messages: []string{"message"}}, nil
			}}
		if err := s2i.NewBuilder(s2i.WithVerbose(verbose), s2i.WithImpl(i), s2i.WithDockerClient(c)).Build(context.Background(), fn.Function{Runtime: "node"}); err != nil {
			t.Fatal(err)
		}
	}
	assert(true)  // when verbose is on, quiet should remain off
	assert(false) // when verbose is off, quiet should be toggled on
}

// Test_BuildEnvs ensures that build environment variables on the Function
// are interpolated and passed to the S2I build implementation in the final
// build config.
func Test_BuildEnvs(t *testing.T) {
	defer WithEnvVar(t, "INTERPOLATE_ME", "interpolated")()
	var (
		envName  = "NAME"
		envValue = "{{ env:INTERPOLATE_ME }}"
		f        = fn.Function{
			Runtime:   "node",
			BuildEnvs: []fn.Env{{Name: &envName, Value: &envValue}},
		}
		i = &mockImpl{}
		c = noopDockerClient{}
		b = s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c))
	)
	i.BuildFn = func(cfg *api.Config) (r *api.Result, err error) {
		for _, v := range cfg.Environment {
			if v.Name == envName && v.Value == "interpolated" {
				return // success!
			} else if v.Name == envName && v.Value == envValue {
				t.Fatal("build env was not interpolated")
			}
		}
		t.Fatal("build envs not added to builder impl config")
		return
	}
	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// mockImpl is a mock implementation of an S2I builder.
type mockImpl struct {
	BuildFn func(*api.Config) (*api.Result, error)
}

func (i *mockImpl) Build(cfg *api.Config) (*api.Result, error) {
	return i.BuildFn(cfg)
}

type noopDockerClient struct{}

func (n noopDockerClient) ImageBuild(ctx context.Context, context io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	_, _ = io.Copy(io.Discard, context)
	return types.ImageBuildResponse{
		Body:   io.NopCloser(strings.NewReader("")),
		OSType: "linux",
	}, nil
}
