package s2i_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/client"

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
		inspect: func(ctx context.Context, image string) (client.ImageInspectResult, error) {
			return client.ImageInspectResult{}, errors.New("this is expected")
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

type mockDocker struct {
	inspect func(ctx context.Context, image string) (client.ImageInspectResult, error)
}

func (m mockDocker) ImageInspect(ctx context.Context, image string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if m.inspect != nil {
		return m.inspect(ctx, image)
	}

	return client.ImageInspectResult{}, nil
}

func (m mockDocker) ImageBuild(ctx context.Context, context io.Reader, options client.ImageBuildOptions) (client.ImageBuildResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerAttach(ctx context.Context, container string, options client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCommit(ctx context.Context, container string, options client.ContainerCommitOptions) (client.ContainerCommitResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerInspect(ctx context.Context, container string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerRemove(ctx context.Context, container string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerStart(ctx context.Context, container string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerKill(ctx context.Context, container string, options client.ContainerKillOptions) (client.ContainerKillResult, error) {
	panic("implement me")
}

func (m mockDocker) ContainerWait(ctx context.Context, container string, options client.ContainerWaitOptions) client.ContainerWaitResult {
	panic("implement me")
}

func (m mockDocker) CopyToContainer(ctx context.Context, container string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	panic("implement me")
}

func (m mockDocker) CopyFromContainer(ctx context.Context, container string, options client.CopyFromContainerOptions) (client.CopyFromContainerResult, error) {
	panic("implement me")
}

func (m mockDocker) ImagePull(ctx context.Context, ref string, options client.ImagePullOptions) (client.ImagePullResponse, error) {
	panic("implement me")
}

func (m mockDocker) ImageRemove(ctx context.Context, image string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	panic("implement me")
}

func (m mockDocker) ServerVersion(ctx context.Context, options client.ServerVersionOptions) (client.ServerVersionResult, error) {
	panic("implement me")
}

// Test_FuncIgnoreSymlinked verifies that .s2iignore is symlinked to
// .funcignore during the build and removed after.
func Test_FuncIgnoreSymlinked(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	f := fn.Function{
		Name:     "test",
		Root:     root,
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	var err error
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	// Write patterns including all supported forms
	content := "# comment\nnotes.txt\n*.tmp\n/docs\ndist/\n"
	if err = os.WriteFile(filepath.Join(root, ".funcignore"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	i := &mockImpl{}
	c := mockDocker{}
	b := s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c))

	s2iignorePath := filepath.Join(root, ".s2iignore")

	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		// During build: .s2iignore should exist and be a symlink to .funcignore
		info, err := os.Lstat(s2iignorePath)
		if err != nil {
			t.Fatalf(".s2iignore not found during build: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatal(".s2iignore should be a symlink")
		}
		target, err := os.Readlink(s2iignorePath)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Clean(target) != filepath.Clean("./.funcignore") {
			t.Fatalf(".s2iignore should point to ./.funcignore, got %q", target)
		}

		// Verify .s2iignore content matches .funcignore (comments included — S2I strips them)
		data, err := os.ReadFile(s2iignorePath)
		if err != nil {
			t.Fatal(err)
		}
		funcignoreData, err := os.ReadFile(filepath.Join(root, ".funcignore"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(funcignoreData) {
			t.Fatal(".s2iignore content does not match .funcignore")
		}
		return nil, nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}

	// After build: .s2iignore should be removed
	if _, err := os.Lstat(s2iignorePath); !os.IsNotExist(err) {
		t.Fatal(".s2iignore should be removed after build")
	}
}

// Test_FuncIgnorePreexistingS2iIgnore verifies that a pre-existing
// .s2iignore is not overwritten and takes precedence.
func Test_FuncIgnorePreexistingS2iIgnore(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	f := fn.Function{Name: "test", Root: root, Runtime: "go", Registry: "example.com/alice"}
	var err error
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(root, ".funcignore"), []byte("notes.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, ".s2iignore"), []byte("other.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	i := &mockImpl{}
	c := mockDocker{}
	b := s2i.NewBuilder(s2i.WithImpl(i), s2i.WithDockerClient(c))

	i.BuildFn = func(cfg *api.Config) (*api.Result, error) {
		// .s2iignore should still contain the original content (not overwritten)
		data, err := os.ReadFile(filepath.Join(root, ".s2iignore"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "other.txt\n" {
			t.Fatalf("pre-existing .s2iignore was overwritten, got: %q", string(data))
		}
		return nil, nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
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
