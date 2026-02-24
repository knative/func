package cmd

// Tests for the builder/deployer registry-based inference and compatibility
// logic wired in cmd/registry.go, cmd/build.go and cmd/deploy.go.
//
// The production code path is:
//  1. clientOptions(runtime) on buildConfig or deployConfig resolves a builder/deployer
//     name (explicit flag > InferBuilder/Deployer > default)
//  2. ValidateBuilderCompatibility / ValidateDeployerCompatibility is called to
//     reject incompatible combinations early.
//
// These tests exercise both paths via the registry directly (unit level) and
// via the cmd execution path where the integration is visible from the outside.

import (
	"errors"
	"testing"

	"knative.dev/func/pkg/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/mock"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/s2i"
	. "knative.dev/func/pkg/testing"
	"knative.dev/func/pkg/wasm"
)

// ---------------------------------------------------------------------------
// Registry-level unit tests — inference
// ---------------------------------------------------------------------------

// TestRegistry_InferBuilder_WasiRuntime verifies that InferBuilder returns
// the wasm builder for every WASI runtime and empty string for traditional ones.
func TestRegistry_InferBuilder_WasiRuntime(t *testing.T) {
	r := newRegistry()

	for _, rt := range wasm.AllWasiRuntimes() {
		got := r.InferBuilder(rt)
		if got != wasm.Builder {
			t.Errorf("InferBuilder(%q) = %q; want %q", rt, got, wasm.Builder)
		}
	}

	for _, rt := range []string{"go", "node", "python", "java", "rust", "quarkus", ""} {
		got := r.InferBuilder(rt)
		if got != "" {
			t.Errorf("InferBuilder(%q) = %q; want empty — traditional runtimes must not be inferred", rt, got)
		}
	}
}

// TestRegistry_InferDeployer_WasiRuntime verifies that InferDeployer returns
// the wasm deployer for WASI runtimes and empty string for traditional ones.
func TestRegistry_InferDeployer_WasiRuntime(t *testing.T) {
	r := newRegistry()

	for _, rt := range wasm.AllWasiRuntimes() {
		got := r.InferDeployer(rt)
		if got != wasm.Deployer {
			t.Errorf("InferDeployer(%q) = %q; want %q", rt, got, wasm.Deployer)
		}
	}

	for _, rt := range []string{"go", "node", "python", "java", "rust", ""} {
		got := r.InferDeployer(rt)
		if got != "" {
			t.Errorf("InferDeployer(%q) = %q; want empty — traditional runtimes must not be inferred", rt, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Registry-level unit tests — compatibility validation
// ---------------------------------------------------------------------------

// TestRegistry_ValidateBuilderCompatibility checks that the registry correctly
// accepts/rejects builder+runtime combinations, and that incompatible errors
// wrap fn.ErrIncompatibility.
func TestRegistry_ValidateBuilderCompatibility(t *testing.T) {
	r := newRegistry()

	tests := []struct {
		runtime string
		builder string
		wantErr error // nil means no error expected; non-nil is matched with errors.Is
	}{
		// Traditional builders accept traditional runtimes
		{runtime: "go", builder: buildpacks.BuilderName, wantErr: nil},
		{runtime: "node", builder: s2i.BuilderName, wantErr: nil},
		{runtime: "python", builder: oci.BuilderName, wantErr: nil},
		// Traditional builders must reject WASI runtimes
		{runtime: wasm.RuntimeRustWasi, builder: buildpacks.BuilderName, wantErr: fn.ErrIncompatibility},
		{runtime: wasm.RuntimeGoWasi, builder: s2i.BuilderName, wantErr: fn.ErrIncompatibility},
		{runtime: wasm.RuntimePythonWasi, builder: oci.BuilderName, wantErr: fn.ErrIncompatibility},
		// WASM builder accepts WASI runtimes
		{runtime: wasm.RuntimeRustWasi, builder: wasm.Builder, wantErr: nil},
		{runtime: wasm.RuntimeGoWasi, builder: wasm.Builder, wantErr: nil},
		// WASM builder must reject traditional runtimes
		{runtime: "go", builder: wasm.Builder, wantErr: fn.ErrIncompatibility},
		{runtime: "node", builder: wasm.Builder, wantErr: fn.ErrIncompatibility},
	}

	for _, tt := range tests {
		t.Run(tt.runtime+"/"+tt.builder, func(t *testing.T) {
			err := r.ValidateBuilderCompatibility(tt.runtime, tt.builder)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error for runtime=%q builder=%q: %v", tt.runtime, tt.builder, err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error wrapping %v for runtime=%q builder=%q, got nil", tt.wantErr, tt.runtime, tt.builder)
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error %v does not wrap %v", err, tt.wantErr)
			}
		})
	}
}

// TestRegistry_ValidateDeployerCompatibility checks that the registry correctly
// accepts/rejects deployer+runtime combinations.
func TestRegistry_ValidateDeployerCompatibility(t *testing.T) {
	r := newRegistry()

	tests := []struct {
		runtime  string
		deployer string
		wantErr  error
	}{
		// Traditional deployers accept traditional runtimes
		{runtime: "go", deployer: knative.KnativeDeployerName, wantErr: nil},
		{runtime: "node", deployer: k8s.KubernetesDeployerName, wantErr: nil},
		{runtime: "python", deployer: keda.KedaDeployerName, wantErr: nil},
		// Traditional deployers must reject WASI runtimes
		{runtime: wasm.RuntimeRustWasi, deployer: knative.KnativeDeployerName, wantErr: fn.ErrIncompatibility},
		{runtime: wasm.RuntimeGoWasi, deployer: k8s.KubernetesDeployerName, wantErr: fn.ErrIncompatibility},
		{runtime: wasm.RuntimePythonWasi, deployer: keda.KedaDeployerName, wantErr: fn.ErrIncompatibility},
		// WASM deployer accepts WASI runtimes
		{runtime: wasm.RuntimeRustWasi, deployer: wasm.Deployer, wantErr: nil},
		{runtime: wasm.RuntimeGoWasi, deployer: wasm.Deployer, wantErr: nil},
		// WASM deployer must reject traditional runtimes
		{runtime: "go", deployer: wasm.Deployer, wantErr: fn.ErrIncompatibility},
		{runtime: "node", deployer: wasm.Deployer, wantErr: fn.ErrIncompatibility},
	}

	for _, tt := range tests {
		t.Run(tt.runtime+"/"+tt.deployer, func(t *testing.T) {
			err := r.ValidateDeployerCompatibility(tt.runtime, tt.deployer)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error for runtime=%q deployer=%q: %v", tt.runtime, tt.deployer, err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error wrapping %v for runtime=%q deployer=%q, got nil", tt.wantErr, tt.runtime, tt.deployer)
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error %v does not wrap %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cmd integration tests — build command
// ---------------------------------------------------------------------------

// TestBuild_IncompatibleBuilderRejectsWasi verifies that attempting to build
// a WASI function with a traditional builder (--builder=pack) returns an error
// wrapping fn.ErrIncompatibility, and the builder is never invoked.
func TestBuild_IncompatibleBuilderRejectsWasi(t *testing.T) {
	root := FromTempDirectory(t)

	// Write function directly — rust-wasi is not in the embedded template repo,
	// so we skip Init and write a minimal func.yaml directly.
	f := fn.Function{
		Root:     root,
		Name:     "mywasifunc",
		Runtime:  wasm.RuntimeRustWasi,
		Registry: TestRegistry,
	}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	builder := mock.NewBuilder()
	cmd := NewBuildCmd(NewTestClient(fn.WithRegistry(TestRegistry), fn.WithBuilder(builder)))
	cmd.SetArgs([]string{"--builder=" + buildpacks.BuilderName})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when using pack builder with a WASI runtime, got nil")
	}
	if !errors.Is(err, fn.ErrIncompatibility) {
		t.Errorf("expected error wrapping fn.ErrIncompatibility, got: %v", err)
	}
	if builder.BuildInvoked {
		t.Error("builder should not be invoked when builder/runtime compatibility fails")
	}
}

// TestBuild_TraditionalRuntimeUsesDefaultBuilder verifies that a traditional
// runtime (e.g. "go") with no explicit --builder flag picks the default
// builder (pack) and does NOT trigger a compatibility error (wasm is not
// inferred for traditional runtimes).
func TestBuild_TraditionalRuntimeUsesDefaultBuilder(t *testing.T) {
	root := FromTempDirectory(t)

	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: TestRegistry,
	}
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	builder := mock.NewBuilder()
	cmd := NewBuildCmd(NewTestClient(fn.WithRegistry(TestRegistry), fn.WithBuilder(builder)))
	cmd.SetArgs([]string{}) // no --builder flag → defaults to pack
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error for traditional runtime without explicit builder: %v", err)
	}
}

// ---------------------------------------------------------------------------
// cmd integration tests — deploy command
// ---------------------------------------------------------------------------

// TestDeploy_IncompatibleDeployerRejectsWasi verifies that attempting to deploy
// a WASI function with a traditional deployer (--deployer=knative) returns an
// error wrapping fn.ErrIncompatibility.
func TestDeploy_IncompatibleDeployerRejectsWasi(t *testing.T) {
	root := FromTempDirectory(t)

	// Write function directly — rust-wasi is not in the embedded template repo.
	f := fn.Function{
		Root:     root,
		Name:     "mywasifunc",
		Runtime:  wasm.RuntimeRustWasi,
		Registry: TestRegistry,
	}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	deployer := mock.NewDeployer()
	builder := mock.NewBuilder()
	cmd := NewDeployCmd(NewTestClient(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(builder),
		fn.WithDeployer(deployer),
	))
	cmd.SetArgs([]string{"--build=false", "--push=false", "--deployer=" + knative.KnativeDeployerName})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when using knative deployer with a WASI runtime, got nil")
	}
	if !errors.Is(err, fn.ErrIncompatibility) {
		t.Errorf("expected error wrapping fn.ErrIncompatibility, got: %v", err)
	}
}

// TestDeploy_TraditionalRuntimeDefaultsToKnative verifies that a traditional
// runtime with no explicit --deployer flag uses knative (the fallback default)
// and does NOT trigger a compatibility error (wasm is not inferred for
// traditional runtimes).
func TestDeploy_TraditionalRuntimeDefaultsToKnative(t *testing.T) {
	root := FromTempDirectory(t)

	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: TestRegistry,
	}
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	deployer := mock.NewDeployer()
	builder := mock.NewBuilder()
	cmd := NewDeployCmd(NewTestClient(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(builder),
		fn.WithDeployer(deployer),
	))
	// No --deployer flag: inference returns "" → fallback knative (no WASI)
	cmd.SetArgs([]string{"--build=false", "--push=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error for traditional runtime without explicit deployer: %v", err)
	}
}
