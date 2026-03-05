package s2i_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	mobyClient "github.com/moby/moby/client"
	"github.com/openshift/source-to-image/pkg/api"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/s2i"
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
	if err := builder.Build(t.Context(), f, nil); err != nil {
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
	if err := b.Build(t.Context(), f, nil); err != nil {
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
			Build(t.Context(), fn.Function{Runtime: "node"}, nil); err != nil {
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
	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// Test_MiddlewareLabel ensures that the middleware-version label is set
// on the S2I build config for runtimes that support scaffolding.
func Test_MiddlewareLabel(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	f := fn.Function{
		Name:     "test-middleware-label",
		Root:     root,
		Runtime:  "go",
		Registry: "example.com/alice",
	}

	var err error
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	i := &mockImpl{}
	c := mockDocker{}
	b := s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c))

	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		// Verify middleware-version label is set
		if cfg.Labels == nil {
			t.Fatal("expected Labels to be set on config")
		}
		middlewareVersion, ok := cfg.Labels[fn.MiddlewareVersionLabelKey]
		if !ok {
			t.Fatalf("expected label %q to be set", fn.MiddlewareVersionLabelKey)
		}
		if middlewareVersion == "" {
			t.Fatalf("expected label %q to have a non-empty value", fn.MiddlewareVersionLabelKey)
		}
		t.Logf("middleware-version label: %s", middlewareVersion)
		return nil, nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFail(t *testing.T) {
	cli := mockDocker{
		inspect: func(ctx context.Context, image string) (mobyClient.ImageInspectResult, error) {
			return mobyClient.ImageInspectResult{}, errors.New("this is expected")
		},
	}
	b := s2i.NewBuilder(s2i.WithDockerClient(cli))
	err := b.Build(t.Context(), fn.Function{Runtime: "node"}, nil)
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

// mockDocker implements s2idocker.Client (openshift/source-to-image v1.6.0+)
// which uses moby/moby/client types.
type mockDocker struct {
	inspect func(ctx context.Context, image string) (mobyClient.ImageInspectResult, error)
}

func (m mockDocker) ImageInspect(ctx context.Context, image string, _ ...mobyClient.ImageInspectOption) (mobyClient.ImageInspectResult, error) {
	if m.inspect != nil {
		return m.inspect(ctx, image)
	}
	return mobyClient.ImageInspectResult{}, nil
}

func (m mockDocker) ImageBuild(ctx context.Context, context io.Reader, options mobyClient.ImageBuildOptions) (mobyClient.ImageBuildResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerAttach(ctx context.Context, container string, options mobyClient.ContainerAttachOptions) (mobyClient.ContainerAttachResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCommit(ctx context.Context, container string, options mobyClient.ContainerCommitOptions) (mobyClient.ContainerCommitResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCreate(ctx context.Context, options mobyClient.ContainerCreateOptions) (mobyClient.ContainerCreateResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerInspect(ctx context.Context, container string, options mobyClient.ContainerInspectOptions) (mobyClient.ContainerInspectResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerRemove(ctx context.Context, container string, options mobyClient.ContainerRemoveOptions) (mobyClient.ContainerRemoveResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerStart(ctx context.Context, container string, options mobyClient.ContainerStartOptions) (mobyClient.ContainerStartResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerKill(ctx context.Context, container string, options mobyClient.ContainerKillOptions) (mobyClient.ContainerKillResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerWait(ctx context.Context, container string, options mobyClient.ContainerWaitOptions) mobyClient.ContainerWaitResult {
	panic("implement me")
}

func (m mockDocker) CopyToContainer(ctx context.Context, container string, options mobyClient.CopyToContainerOptions) (mobyClient.CopyToContainerResult, error) {
	panic("implement me")
}

func (m mockDocker) CopyFromContainer(ctx context.Context, container string, options mobyClient.CopyFromContainerOptions) (mobyClient.CopyFromContainerResult, error) {
	panic("implement me")
}

func (m mockDocker) ImagePull(ctx context.Context, ref string, options mobyClient.ImagePullOptions) (mobyClient.ImagePullResponse, error) {
	panic("implement me")
}

func (m mockDocker) ImageRemove(ctx context.Context, image string, options mobyClient.ImageRemoveOptions) (mobyClient.ImageRemoveResult, error) {
	panic("implement me")
}

func (m mockDocker) ServerVersion(ctx context.Context, options mobyClient.ServerVersionOptions) (mobyClient.ServerVersionResult, error) {
	panic("implement me")
}

// Test_ScaffoldWritesToFuncBuild ensures that scaffolding for Go/Python
// runtimes is written to .func/build/ instead of .s2i/build/
func Test_ScaffoldWritesToFuncBuild(t *testing.T) {
	runtimes := []string{"go", "python"}
	for _, rt := range runtimes {
		t.Run(rt, func(t *testing.T) {
			var (
				root, done = Mktemp(t)
				f          = fn.Function{
					Name:     "test",
					Root:     root,
					Runtime:  rt,
					Registry: "example.com/alice",
				}
				scaffolder = s2i.NewScaffolder(false)
				err        error
			)
			defer done()

			// Initialize the test function
			if f, err = fn.New().Init(f); err != nil {
				t.Fatal(err)
			}

			// Call Scaffold
			if err := scaffolder.Scaffold(t.Context(), f, ""); err != nil {
				t.Fatal(err)
			}

			// Assert: scaffolding should be in .func/build/
			expectedPath := filepath.Join(root, fn.RunDataDir, fn.BuildDir)
			if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
				t.Errorf("expected scaffolding at %s, but directory does not exist", expectedPath)
			}

			// Assert: S2I scripts should be at .func/build/bin/
			scriptsPath := filepath.Join(root, fn.RunDataDir, fn.BuildDir, "bin", "assemble")
			if _, err := os.Stat(scriptsPath); os.IsNotExist(err) {
				t.Errorf("expected assemble script at %s, but file does not exist", scriptsPath)
			}

			// Assert: .s2i directory should NOT exist at root level
			s2iPath := filepath.Join(root, ".s2i")
			if _, err := os.Stat(s2iPath); err == nil {
				t.Errorf(".s2i directory should not exist at root level, but found at %s", s2iPath)
			}
		})
	}
}
