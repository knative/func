package buildpacks

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/pkg/errors"
	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
)

// TestBuild_BuilderImageUntrusted ensures that only known builder images
// are to be considered trusted.
func TestBuild_BuilderImageUntrusted(t *testing.T) {
	var untrusted = []string{
		// Check prefixes that end in a slash
		"quay.io/bosonhack/",
		"gcr.io/paketo-buildpackshack/",
		// And those that don't
		"docker.io/paketobuildpackshack",
	}

	for _, builder := range untrusted {
		if TrustBuilder(builder) {
			t.Fatalf("expected pack builder image %v to be untrusted", builder)
		}
	}
}

// TestBuild_BuilderImageTrusted ensures that only known builder images
// are to be considered trusted.
func TestBuild_BuilderImageTrusted(t *testing.T) {
	for _, builder := range trustedBuilderImagePrefixes {
		if !TrustBuilder(builder) {
			t.Fatalf("expected pack builder image %v to be trusted", builder)
		}
	}
}

// TestBuild_BuilderImageDefault ensures that a Function bing built which does not
// define a Builder Image will get the internally-defined default.
func TestBuild_BuilderImageDefault(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "node"}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		expected := DefaultBuilderImages["node"]
		if opts.Builder != expected {
			t.Fatalf("expected pack builder image '%v', got '%v'", expected, opts.Builder)
		}
		return nil
	}

	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}

}

// TestBuild_BuildpacksDefault ensures that, if there are default buildpacks
// defined in-code, but none defined on the function, the defaults will be
// used.
func TestBuild_BuildpacksDefault(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "go"}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		expected := defaultBuildpacks["go"]
		if !reflect.DeepEqual(expected, opts.Buildpacks) {
			t.Fatalf("expected buildpacks '%v', got '%v'", expected, opts.Buildpacks)
		}
		return nil
	}

	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}

}

// TestBuild_BuilderImageConfigurable ensures that the builder will use the builder
// image defined on the given Function if provided.
func TestBuild_BuilderImageConfigurable(t *testing.T) {
	var (
		i = &mockImpl{} // mock underlying implementation
		b = NewBuilder( // Func Builder logic
			WithName(builders.Pack), WithImpl(i))
		f = fn.Function{ // Function with a builder image set
			Runtime: "node",
			Build: fn.BuildSpec{
				BuilderImages: map[string]string{
					builders.Pack: "example.com/user/builder-image",
				},
			},
		}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		expected := "example.com/user/builder-image"
		if opts.Builder != expected {
			t.Fatalf("expected builder image for node to be '%v', got '%v'", expected, opts.Builder)
		}
		return nil
	}

	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_BuilderImageExclude ensures that ignored files are not added to the func
// image
func TestBuild_BuilderImageExclude(t *testing.T) {
	var (
		i = &mockImpl{} // mock underlying implementation
		b = NewBuilder( // Func Builder logic
			WithName(builders.Pack), WithImpl(i))
		f = fn.Function{
			Runtime: "go",
		}
	)
	funcIgnoreContent := []byte(`#testing comments
hello.txt`)
	expected := []string{"hello.txt"}

	tempdir := t.TempDir()
	f.Root = tempdir

	//create a .funcignore file containing the details of the files to be ignored
	err := os.WriteFile(filepath.Join(f.Root, ".funcignore"), funcIgnoreContent, 0644)
	if err != nil {
		t.Fatal(err)
	}

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		if len(opts.ProjectDescriptor.Build.Exclude) != 2 {
			t.Fatalf("expected 2 lines of exclusions , got %v", len(opts.ProjectDescriptor.Build.Exclude))
		}
		if opts.ProjectDescriptor.Build.Exclude[1] != expected[0] {
			t.Fatalf("expected excluded file to be '%v', got '%v'", expected[1], opts.ProjectDescriptor.Build.Exclude[1])
		}
		return nil
	}

	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_Envs ensures that build environment variables are interpolated and
// provided in Build Options
func TestBuild_Envs(t *testing.T) {
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
		b = NewBuilder(WithImpl(i))
	)
	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		for k, v := range opts.Env {
			if k == envName && v == "interpolated" {
				return nil // success!
			} else if k == envName && v == envValue {
				t.Fatal("build env was not interpolated")
			}
		}
		t.Fatal("build envs not added to builder options")
		return nil
	}
	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_Errors confirms error scenarios.
func TestBuild_Errors(t *testing.T) {
	testCases := []struct {
		name, runtime, expectedErr string
	}{
		{name: "test runtime required error", expectedErr: "Pack requires the Function define a language runtime"},
		{
			name:        "test runtime not supported error",
			runtime:     "test-runtime-language",
			expectedErr: "Pack builder has no default builder image for the 'test-runtime-language' language runtime.  Please provide one.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
			t.Parallel()
			gotErr := ErrRuntimeRequired{}.Error()
			if tc.runtime != "" {
				gotErr = ErrRuntimeNotSupported{Runtime: tc.runtime}.Error()
			}

			if tc.expectedErr != gotErr {
				t.Fatalf("Unexpected error want:\n%v\ngot:\n%v", tc.expectedErr, gotErr)
			}
		})
	}
}

// TestBuild_HomeAndPermissions ensures that function fails during build while HOME is
// not defined and different .config permission scenarios
func TestBuild_HomeAndPermissions(t *testing.T) {
	testCases := []struct {
		name        string
		homePath    bool
		homePerms   fs.FileMode
		expectedErr bool
		errContains string
	}{
		{name: "xpct-fail - create pack client when HOME not defined", homePath: false, homePerms: 0, expectedErr: true, errContains: "$HOME is not defined"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
			var (
				i = &mockImpl{}
				b = NewBuilder( // Func Builder logic
					WithName(builders.Pack),
					WithImpl(i))
				f = fn.Function{
					Runtime: "go",
					Build:   fn.BuildSpec{Image: "example.com/parent/name"},
				}
			)
			// set temporary dir and assign it as func directory
			tempdir := t.TempDir()
			f.Root = tempdir

			// setup HOME env and its perms
			if tc.homePath == false {
				t.Setenv("HOME", "")
			} else {
				// setup new home with perms
				err := os.MkdirAll(filepath.Join(tempdir, "home"), tc.homePerms)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("HOME", filepath.Join(tempdir, "home"))
			}
			// TODO: gauron99 -- add test for pack_home?
			i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
				packHome := os.Getenv("PACK_HOME")
				if packHome == "" {
					_, err := os.UserHomeDir()
					if err != nil {
						return errors.Wrap(err, "getting user home")
					}
				}
				return nil
			}

			err := b.Build(context.Background(), f, nil)
			// error scenarios
			if tc.expectedErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				} else if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error message '%v' was not found in the actual error: '%v'", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("didnt expect an error but got '%v'", err)
				}
			}
		})
	}
}

type mockImpl struct {
	BuildFn func(context.Context, pack.BuildOptions) error
}

func (i mockImpl) Build(ctx context.Context, opts pack.BuildOptions) error {
	return i.BuildFn(ctx, opts)
}
