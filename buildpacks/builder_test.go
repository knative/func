package buildpacks

import (
	"context"
	"testing"

	pack "github.com/buildpacks/pack/pkg/client"
	fn "knative.dev/func"
	"knative.dev/func/builders"
)

// Test_BuilderImageTrusted ensures that only known builder images
// are to be considered trusted.
func Test_BuilderImageTrusted(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "node"}
	)

	for _, builder := range trustedBuilderImagePrefixes {
		f.Build = fn.BuildSpec{
			BuilderImages: map[string]string{
				builders.Pack: builder,
			},
		}
		i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
			if opts.TrustBuilder("") != true {
				t.Fatalf("expected pack builder image %v to be trusted", f.Build.BuilderImages[builders.Pack])
			}
			return nil
		}

		if err := b.Build(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
}

// Test_BuilderImageDefault ensures that a Function bing built which does not
// define a Builder Image will get the internally-defined default.
func Test_BuilderImageDefault(t *testing.T) {
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

	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}

}

// Test_BuilderImageConfigurable ensures that the builder will use the builder
// image defined on the given Function if provided.
func Test_BuilderImageConfigurable(t *testing.T) {
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

	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// Test_BuildEnvs ensures that build environment variables are interpolated and
// provided in Build Options
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
	if err := b.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

func Test_BuildErrors(t *testing.T) {
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
