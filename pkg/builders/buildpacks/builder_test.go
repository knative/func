package buildpacks

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	pack "github.com/buildpacks/pack/pkg/client"
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

type mockImpl struct {
	BuildFn func(context.Context, pack.BuildOptions) error
}

func (i mockImpl) Build(ctx context.Context, opts pack.BuildOptions) error {
	return i.BuildFn(ctx, opts)
}
