package s2i_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/openshift/source-to-image/pkg/api"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// Test_BuildImages ensures that supported runtimes returns builder image
func Test_BuildImages(t *testing.T) {

	tests := []struct {
		name     string
		function fn.Function
		wantErr  bool
	}{
		{
			name:     "Without builder - without runtime",
			function: fn.Function{},
			wantErr:  true,
		},
		{
			name:     "Without builder - supported runtime - node",
			function: fn.Function{Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Without builder - supported runtime - typescript",
			function: fn.Function{Runtime: "typescript"},
			wantErr:  false,
		},
		{
			name:     "Without builder - supported runtime - quarkus",
			function: fn.Function{Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Without builder - supported runtime - go",
			function: fn.Function{Runtime: "go"},
			wantErr:  false,
		},
		{
			name:     "Without builder - supported runtime - python",
			function: fn.Function{Runtime: "python"},
			wantErr:  false,
		},
		{
			name:     "Without builder - unsupported runtime - rust",
			function: fn.Function{Runtime: "rust"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s2i.BuilderImage(tt.function, builders.S2I)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuilderImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

// Test_BuilderImageDefault ensures that a function being built which does not
// define a Builder Image will default.
func Test_BuilderImageDefault(t *testing.T) {
	var (
		root, done = Mktemp(t)
		runtime    = "go"
		impl       = &mockImpl{} // mock the underlying s2i implementation
		f          = fn.Function{
			Name:     "test",
			Root:     root,
			Runtime:  runtime,
			Registry: "example.com/alice"} // function with no builder image set
		builder = s2i.NewBuilder( // func S2I Builder logic
			s2i.WithImpl(impl),
			s2i.WithDockerClient(mockDocker{}))
		err error
	)
	defer done()

	// Initialize the test function
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	// An implementation of the underlying S2I builder which verifies
	// the config has arrived as expected (correct functions logic applied)
	impl.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		expected := s2i.DefaultBuilderImages[runtime]
		if cfg.BuilderImage != expected {
			t.Fatalf("expected s2i config builder image '%v', got '%v'",
				expected, cfg.BuilderImage)
		}
		return nil, nil
	}

	// Invoke Build, which runs function Builder logic before invoking the
	// mock impl above.
	if err := builder.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// Test_BuilderImageConfigurable ensures that the builder will use the builder
// image defined on the given function if provided.
func Test_BuilderImageConfigurable(t *testing.T) {
	var (
		i = &mockImpl{}     // mock underlying s2i implementation
		c = mockDocker{}    // mock docker client
		b = s2i.NewBuilder( // func S2I Builder logic
			s2i.WithName(builders.S2I), s2i.WithImpl(i), s2i.WithDockerClient(c))
		f = fn.Function{ // function with a builder image set
			Runtime: "node",
			Build: fn.BuildSpec{
				BuilderImages: map[string]string{
					builders.S2I: "example.com/user/builder-image",
				},
			},
		}
	)

	// An implementation of the underlying S2I implementation which verifies
	// the config has arrived as expected (correct functions logic applied)
	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		expected := "example.com/user/builder-image"
		if cfg.BuilderImage != expected {
			t.Fatalf("expected s2i config builder image for node to be '%v', got '%v'", expected, cfg.BuilderImage)
		}
		return nil, nil
	}

	// Invoke Build, which runs function Builder logic before invoking the
	// mock impl above.
	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// Test_Verbose ensures that the verbosity flag is propagated to the
// S2I builder implementation.
func Test_BuilderVerbose(t *testing.T) {
	c := mockDocker{} // mock docker client
	assert := func(verbose bool) {
		i := &mockImpl{
			BuildFn: func(cfg *api.Config) (r *api.Result, err error) {
				if cfg.Quiet == verbose {
					t.Fatalf("expected s2i quiet mode to be !%v when verbose %v", verbose, verbose)
				}
				return &api.Result{Messages: []string{"message"}}, nil
			}}
		if err := s2i.NewBuilder(s2i.WithVerbose(verbose), s2i.WithImpl(i), s2i.WithDockerClient(c)).
			Build(context.Background(), fn.Function{Runtime: "node"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	assert(true)  // when verbose is on, quiet should remain off
	assert(false) // when verbose is off, quiet should be toggled on
}

// Test_BuildEnvs ensures that build environment variables on the function
// are interpolated and passed to the S2I build implementation in the final
// build config.
func Test_BuildEnvs(t *testing.T) {
	t.Setenv("INTERPOLATE_ME", "interpolated")
	var (
		envName  = "NAME"
		envValue = "{{ env:INTERPOLATE_ME }}"
		f        = fn.Function{
			Runtime: "node",
			Build: fn.BuildSpec{
				BuildEnvs: []fn.Env{{Name: &envName, Value: &envValue}},
			},
		}
		i = &mockImpl{}
		c = mockDocker{}
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
	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFail(t *testing.T) {
	cli := mockDocker{
		inspect: func(ctx context.Context, image string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, errors.New("this is expected")
		},
	}
	b := s2i.NewBuilder(s2i.WithDockerClient(cli))
	err := b.Build(context.Background(), fn.Function{Runtime: "node"}, nil)
	if err == nil {
		t.Error("didn't get expected error")
	}
}

// mockImpl is a mock implementation of an S2I builder.
type mockImpl struct {
	BuildFn func(*api.Config) (*api.Result, error)
}

func (i *mockImpl) Build(cfg *api.Config) (*api.Result, error) {
	return i.BuildFn(cfg)
}

type mockDocker struct {
	inspect func(ctx context.Context, image string) (types.ImageInspect, []byte, error)
}

func (m mockDocker) ImageInspectWithRaw(ctx context.Context, image string) (types.ImageInspect, []byte, error) {
	if m.inspect != nil {
		return m.inspect(ctx, image)
	}

	return types.ImageInspect{}, nil, nil
}

func (m mockDocker) ImageBuild(ctx context.Context, context io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	panic("implement me")
}

func (m mockDocker) ContainerAttach(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCommit(ctx context.Context, container string, options container.CommitOptions) (types.IDResponse, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error) {
	panic("implement me")
}

func (m mockDocker) ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error) {
	panic("implement me")
}

func (m mockDocker) ContainerRemove(ctx context.Context, container string, options container.RemoveOptions) error {
	panic("implement me")
}

func (m mockDocker) ContainerStart(ctx context.Context, container string, options container.StartOptions) error {
	panic("implement me")
}

func (m mockDocker) ContainerKill(ctx context.Context, container, signal string) error {
	panic("implement me")
}

func (m mockDocker) ContainerWait(ctx context.Context, container string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	panic("implement me")
}

func (m mockDocker) CopyToContainer(ctx context.Context, container, path string, content io.Reader, opts container.CopyToContainerOptions) error {
	panic("implement me")
}

func (m mockDocker) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, container.PathStat, error) {
	panic("implement me")
}

func (m mockDocker) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	panic("implement me")
}

func (m mockDocker) ImageRemove(ctx context.Context, image string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	panic("implement me")
}

func (m mockDocker) ServerVersion(ctx context.Context) (types.Version, error) {

	panic("implement me")
}

type notFoundErr struct {
}

func (n notFoundErr) Error() string {
	return "not found"
}

func (n notFoundErr) NotFound() {
	panic("just a marker interface")
}

// Just a type assert in case docker decides to change NotFoundError interface again
var _ errdefs.ErrNotFound = notFoundErr{}
