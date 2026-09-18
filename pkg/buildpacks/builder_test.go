package buildpacks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pack "github.com/buildpacks/pack/pkg/client"
	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/scaffolding"
	. "knative.dev/func/pkg/testing"
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

func TestBuild_BuilderImageTrustedLocalhost(t *testing.T) {
	for _, reg := range []string{
		"localhost",
		"localhost:5000",
		"127.0.0.1",
		"127.0.0.1:5000",
		"[::1]",
		"[::1]:5000"} {
		t.Run(reg, func(t *testing.T) {
			if !TrustBuilder(reg + "/project/builder:latest") {
				t.Errorf("expected to be trusted: %q", reg)
			}
		})
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

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}

}

// TestBuild_BuildpacksDefault ensures that, if there are default buildpacks
// defined in-code, but none defined on the function, the defaults will be
// used.
func TestBuild_BuildpacksDefault(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	var (
		i   = &mockImpl{}
		b   = NewBuilder(WithImpl(i))
		f   = fn.Function{Runtime: "go", Root: root, Registry: "example.com/alice"}
		err error
	)

	// Initialize the function to create proper source files
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		expected := defaultBuildpacks["go"]
		if !reflect.DeepEqual(expected, opts.Buildpacks) {
			t.Fatalf("expected buildpacks '%v', got '%v'", expected, opts.Buildpacks)
		}
		return nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
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

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_BuilderImageExcludePatterns verifies that all supported
// .funcignore pattern forms are correctly passed to pack's Exclude option.
func TestBuild_BuilderImageExcludePatterns(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	var (
		i   = &mockImpl{}
		b   = NewBuilder(WithName(builders.Pack), WithImpl(i))
		f   = fn.Function{Runtime: "go", Root: root, Registry: "example.com/alice"}
		err error
	)

	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	content := []byte("# comment stripped\nnotes.txt\n*.tmp\n/docs\ndist/\n")
	if err = os.WriteFile(filepath.Join(f.Root, ".funcignore"), content, 0644); err != nil {
		t.Fatal(err)
	}

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		excludes := opts.ProjectDescriptor.Build.Exclude
		// 4 user patterns: notes.txt, *.tmp, /docs, dist/ (comment stripped)
		if len(excludes) != 4 {
			t.Fatalf("expected 4 exclusions, got %v: %v", len(excludes), excludes)
		}
		want := map[string]bool{"notes.txt": true, "*.tmp": true, "/docs": true, "dist/": true}
		for _, e := range excludes {
			if !want[e] {
				t.Errorf("unexpected exclusion: %q", e)
			}
		}
		// Verify comment was stripped
		for _, e := range excludes {
			if len(e) > 0 && e[0] == '#' {
				t.Errorf("comment line in excludes: %q", e)
			}
		}
		return nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
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
	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_MiddlewareLabel ensures that the middleware-version label is set
// on the build options for runtimes that support scaffolding.
func TestBuild_MiddlewareLabel(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{
			Name:     "test-middleware-label",
			Root:     root,
			Runtime:  "go",
			Registry: "example.com/alice",
		}
		err error
	)

	// Initialize the function to create proper source files
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	// Get expected middleware version
	expectedVersion, err := scaffolding.MiddlewareVersion(f.Root, f.Runtime, f.Invoke, fn.EmbeddedTemplatesFS)
	if err != nil {
		t.Fatalf("failed to get expected middleware version: %v", err)
	}
	if expectedVersion == "" {
		t.Fatal("expected middleware version to be non-empty for go runtime")
	}

	expectedLabel := fmt.Sprintf("%s=%s", fn.MiddlewareVersionLabelKey, expectedVersion)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		bpLabels, ok := opts.Env["BP_IMAGE_LABELS"]
		if !ok {
			t.Fatal("expected BP_IMAGE_LABELS to be set")
		}
		if bpLabels != expectedLabel {
			t.Fatalf("expected BP_IMAGE_LABELS to be %q, got: %q", expectedLabel, bpLabels)
		}
		t.Logf("BP_IMAGE_LABELS: %s", bpLabels)
		return nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
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

// TestBuild_PlatformSingle ensures that when a single platform is specified,
// opts.Platform is set correctly on the pack BuildOptions.
func TestBuild_PlatformSingle(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "node"}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		expected := "linux/amd64"
		if opts.Platform != expected {
			t.Fatalf("expected platform '%v', got '%v'", expected, opts.Platform)
		}
		return nil
	}

	platforms := []fn.Platform{{OS: "linux", Architecture: "amd64"}}
	if err := b.Build(t.Context(), f, platforms); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_PlatformMultiple ensures that specifying more than one platform
// returns an error.
func TestBuild_PlatformMultiple(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "node"}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		t.Fatal("build should not have been invoked")
		return nil
	}

	platforms := []fn.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm64"},
	}
	err := b.Build(t.Context(), f, platforms)
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
	expected := "the pack builder currently only supports specifying a single target platform"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

// TestBuild_PlatformNone ensures that passing no platforms still works
// and opts.Platform is empty.
func TestBuild_PlatformNone(t *testing.T) {
	var (
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
		f = fn.Function{Runtime: "node"}
	)

	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		if opts.Platform != "" {
			t.Fatalf("expected empty platform, got '%v'", opts.Platform)
		}
		return nil
	}

	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
}

type mockImpl struct {
	BuildFn func(context.Context, pack.BuildOptions) error
}

func (i mockImpl) Build(ctx context.Context, opts pack.BuildOptions) error {
	return i.BuildFn(ctx, opts)
}

// TestBuild_BuildCACertFile tests the CA certificate file handling
func TestBuild_BuildCACertFile(t *testing.T) {
	tests := []struct {
		name          string
		caCertPath    string
		setupFunc     func(t *testing.T, root string) string
		expectError   bool
		errorContains string
	}{
		{
			name:       "no CA cert file",
			caCertPath: "",
			setupFunc: func(t *testing.T, root string) string {
				return ""
			},
			expectError: false,
		},
		{
			name:       "absolute path to CA cert",
			caCertPath: "", // Will be set by setupFunc
			setupFunc: func(t *testing.T, root string) string {
				// Create a temporary CA cert file
				tmpFile, err := os.CreateTemp("", "ca-cert-*.crt")
				if err != nil {
					t.Fatal(err)
				}
				defer tmpFile.Close()

				// Write some dummy CA cert content
				if _, err := tmpFile.WriteString("-----BEGIN CERTIFICATE-----\nDUMMY\n-----END CERTIFICATE-----\n"); err != nil {
					t.Fatal(err)
				}

				return tmpFile.Name()
			},
			expectError: false,
		},
		{
			name:       "relative path to CA cert",
			caCertPath: "ca-cert.crt",
			setupFunc: func(t *testing.T, root string) string {
				// Don't create the file yet - will be created after Init
				return "ca-cert.crt" // Return relative path
			},
			expectError: false,
		},
		{
			name:          "non-existent CA cert file",
			caCertPath:    "/nonexistent/ca-cert.crt",
			setupFunc:     func(t *testing.T, root string) string { return "/nonexistent/ca-cert.crt" },
			expectError:   true,
			errorContains: "CA bundle file not found",
		},
		{
			name:       "CA cert outside function directory",
			caCertPath: "", // Will be set by setupFunc
			setupFunc: func(t *testing.T, root string) string {
				// Create a temporary CA cert file outside function root
				tmpDir := t.TempDir()
				caPath := filepath.Join(tmpDir, "external-ca.crt")
				if err := os.WriteFile(caPath, []byte("-----BEGIN CERTIFICATE-----\nDUMMY\n-----END CERTIFICATE-----\n"), 0644); err != nil {
					t.Fatal(err)
				}
				return caPath
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, done := Mktemp(t)
			defer done()

			// Setup the CA cert file
			caCertPath := tt.setupFunc(t, root)
			if tt.caCertPath != "" && caCertPath == "" {
				caCertPath = tt.caCertPath
			}

			var (
				i = &mockImpl{}
				b = NewBuilder(WithImpl(i))
				f = fn.Function{
					Runtime:  "go",
					Root:     root,
					Registry: "example.com/alice",
					Build: fn.BuildSpec{
						BuildCACertFile: caCertPath,
					},
				}
			)

			// Initialize the function
			var err error
			if f, err = fn.New().Init(f); err != nil {
				t.Fatal(err)
			}

			// Set the CA cert path after init
			f.Build.BuildCACertFile = caCertPath

			// For relative path test, create the file AFTER init
			if caCertPath == "ca-cert.crt" {
				caPath := filepath.Join(root, "ca-cert.crt")
				if err := os.WriteFile(caPath, []byte("-----BEGIN CERTIFICATE-----\nDUMMY\n-----END CERTIFICATE-----\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Track whether Build was called
			buildCalled := false
			i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
				buildCalled = true

				// If CA cert is provided, verify the binding is set up correctly
				if caCertPath != "" && !tt.expectError {
					// Check that SERVICE_BINDING_ROOT is set
					if _, ok := opts.Env["SERVICE_BINDING_ROOT"]; !ok {
						t.Error("SERVICE_BINDING_ROOT not set when CA cert is provided")
					}

					// Check that volumes are configured
					if len(opts.ContainerConfig.Volumes) == 0 {
						t.Error("No volumes configured when CA cert is provided")
					}

					// Verify at least one volume contains "ca-certificates"
					foundCABinding := false
					for _, vol := range opts.ContainerConfig.Volumes {
						if strings.Contains(vol, "ca-certificates") {
							foundCABinding = true
							break
						}
					}
					if !foundCABinding {
						t.Error("CA certificates binding not found in volumes")
					}
				}

				return nil
			}

			// Execute the build
			err = b.Build(t.Context(), f, nil)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				// Build should not have been called if there was an error
				if buildCalled {
					t.Error("Build was called despite error in setup")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Build should have been called
				if !buildCalled {
					t.Error("Build was not called")
				}
			}

			// Cleanup: remove temporary CA cert files created outside function root
			if caCertPath != "" && filepath.IsAbs(caCertPath) && !strings.HasPrefix(caCertPath, root) {
				os.Remove(caCertPath)
			}
		})
	}
}
