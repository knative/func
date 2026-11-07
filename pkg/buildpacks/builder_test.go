package buildpacks

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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

// writeCAFile writes a PEM bundle of n self-signed CA certificates to path.
func writeCAFile(t *testing.T, path string, n int) {
	t.Helper()
	var buf bytes.Buffer
	for i := 0; i < n; i++ {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(int64(i + 1)),
			Subject:               pkix.Name{CommonName: fmt.Sprintf("test-ca-%d", i)},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		if err = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// caBindingDir returns the host directory mounted at dest in opts.
func caBindingDir(t *testing.T, opts pack.BuildOptions, dest string) string {
	t.Helper()
	suffix := ":" + dest + ":ro"
	for _, v := range opts.ContainerConfig.Volumes {
		if strings.HasSuffix(v, suffix) {
			return strings.TrimSuffix(v, suffix)
		}
	}
	t.Fatalf("no volume mounted at %q in %v", dest, opts.ContainerConfig.Volumes)
	return ""
}

// TestBuild_CAFile ensures a CA file is provided to the build as a Paketo
// ca-certificates binding at the default binding root, one certificate per
// file, and that the binding directory is removed after the build.
func TestBuild_CAFile(t *testing.T) {
	root := t.TempDir()
	writeCAFile(t, filepath.Join(root, "bundle.pem"), 3)

	var (
		i       = &mockImpl{}
		b       = NewBuilder(WithImpl(i))
		f       = fn.Function{Runtime: "node", Root: root, Build: fn.BuildSpec{CAFile: "bundle.pem"}}
		bindDir string
	)
	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		if got := opts.Env["SERVICE_BINDING_ROOT"]; got != "/platform/bindings" {
			t.Errorf("expected SERVICE_BINDING_ROOT /platform/bindings, got %q", got)
		}
		bindDir = caBindingDir(t, opts, "/platform/bindings/func-build-ca")

		typ, err := os.ReadFile(filepath.Join(bindDir, "type"))
		if err != nil {
			t.Fatal(err)
		}
		if string(typ) != "ca-certificates" {
			t.Errorf("expected binding type ca-certificates, got %q", typ)
		}
		entries, err := os.ReadDir(bindDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 4 { // type + 3 certificates
			t.Fatalf("expected 4 files in binding, got %v", entries)
		}
		for n := 0; n < 3; n++ {
			data, err := os.ReadFile(filepath.Join(bindDir, fmt.Sprintf("ca-%d.pem", n)))
			if err != nil {
				t.Fatal(err)
			}
			block, rest := pem.Decode(data)
			if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
				t.Fatalf("ca-%d.pem must hold exactly one certificate", n)
			}
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("test-ca-%d", n); c.Subject.CommonName != want {
				t.Errorf("ca-%d.pem: expected %q, got %q", n, want, c.Subject.CommonName)
			}
		}
		return nil
	}
	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bindDir); !os.IsNotExist(err) {
		t.Fatalf("expected binding dir %q to be removed after build, got %v", bindDir, err)
	}
}

// TestBuild_CAFileBindingRoot ensures a user-defined SERVICE_BINDING_ROOT is
// respected, the user's own mounts are kept and f.Build.Mounts is unchanged.
// It also uses an absolute CA file path outside of the function directory.
func TestBuild_CAFileBindingRoot(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	writeCAFile(t, caFile, 1)

	var (
		name   = "SERVICE_BINDING_ROOT"
		value  = "/bindings"
		mounts = []fn.MountSpec{{Source: "/tmp/git-creds", Destination: "/bindings/git-binding"}}
		f      = fn.Function{
			Runtime: "node",
			Root:    t.TempDir(),
			Build: fn.BuildSpec{
				CAFile:    caFile,
				BuildEnvs: []fn.Env{{Name: &name, Value: &value}},
				Mounts:    slices.Clone(mounts),
			},
		}
		i = &mockImpl{}
		b = NewBuilder(WithImpl(i))
	)
	i.BuildFn = func(ctx context.Context, opts pack.BuildOptions) error {
		if got := opts.Env["SERVICE_BINDING_ROOT"]; got != "/bindings" {
			t.Errorf("expected SERVICE_BINDING_ROOT /bindings, got %q", got)
		}
		if !slices.Contains(opts.ContainerConfig.Volumes, "/tmp/git-creds:/bindings/git-binding") {
			t.Errorf("user mount missing from %v", opts.ContainerConfig.Volumes)
		}
		dir := caBindingDir(t, opts, "/bindings/func-build-ca")
		if _, err := os.Stat(filepath.Join(dir, "ca-0.pem")); err != nil {
			t.Error(err)
		}
		return nil
	}
	if err := b.Build(t.Context(), f, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Build.Mounts, mounts) {
		t.Fatalf("f.Build.Mounts was modified: %v", f.Build.Mounts)
	}
}

// TestBuild_CAFileErrors ensures an unusable CA file fails the build before
// the pack build is started.
func TestBuild_CAFileErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.pem"), []byte("not a certificate\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bad := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")})
	if err := os.WriteFile(filepath.Join(root, "bad.pem"), bad, 0644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, file string }{
		{"missing", "missing.pem"},
		{"no certificates", "empty.pem"},
		{"invalid certificate", "bad.pem"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := &mockImpl{BuildFn: func(context.Context, pack.BuildOptions) error {
				t.Fatal("pack build must not run with an unusable CA file")
				return nil
			}}
			f := fn.Function{Runtime: "node", Root: root, Build: fn.BuildSpec{CAFile: tt.file}}
			err := NewBuilder(WithImpl(i)).Build(t.Context(), f, nil)
			if err == nil || !strings.Contains(err.Error(), tt.file) {
				t.Fatalf("expected an error naming %q, got %v", tt.file, err)
			}
		})
	}
}
