package wasm_test

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/wasm"
)

// newWasmRegistry returns a registry pre-populated with all WASM registrations.
func newWasmRegistry() *fn.Registry {
	r := fn.NewRegistry()
	wasm.Register(r)
	return r
}

// TestInferBuilder_ViaRegistry verifies that InferBuilder returns the
// appropriate builder for WASI and traditional runtimes through the registry.
func TestInferBuilder_ViaRegistry(t *testing.T) {
	tests := []struct {
		runtime string
		want    string
	}{
		// WASI runtimes -> wasm builder
		{wasm.RuntimeRustWasi, wasm.Builder},
		{wasm.RuntimeGoWasi, wasm.Builder},
		{wasm.RuntimePythonWasi, wasm.Builder},
		{wasm.RuntimeJsWasi, wasm.Builder},
		{wasm.RuntimeCWasi, wasm.Builder},
		{wasm.RuntimeCppWasi, wasm.Builder},
		{wasm.RuntimeDotnetWasi, wasm.Builder},
		{wasm.RuntimeSwiftWasi, wasm.Builder},
		// Traditional runtimes -> no inference (empty)
		{"go", ""},
		{"node", ""},
		{"python", ""},
		{"rust", ""},
		{"", ""},
	}

	r := newWasmRegistry()
	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			got := r.InferBuilder(tt.runtime)
			if got != tt.want {
				t.Errorf("InferBuilder(%q) = %q, want %q", tt.runtime, got, tt.want)
			}
		})
	}
}

// TestInferDeployer_ViaRegistry verifies that InferDeployer returns the
// appropriate deployer for WASI and traditional runtimes through the registry.
func TestInferDeployer_ViaRegistry(t *testing.T) {
	tests := []struct {
		runtime string
		want    string
	}{
		// WASI runtimes -> wasm deployer
		{wasm.RuntimeRustWasi, wasm.Deployer},
		{wasm.RuntimeGoWasi, wasm.Deployer},
		{wasm.RuntimePythonWasi, wasm.Deployer},
		// Traditional runtimes -> no inference (empty)
		{"go", ""},
		{"node", ""},
		{"python", ""},
		{"", ""},
	}

	r := newWasmRegistry()
	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			got := r.InferDeployer(tt.runtime)
			if got != tt.want {
				t.Errorf("InferDeployer(%q) = %q, want %q", tt.runtime, got, tt.want)
			}
		})
	}
}

// TestWasmBuilderAcceptsOnlyWasi verifies that the wasm builder registration
// only supports WASI runtimes.
func TestWasmBuilderAcceptsOnlyWasi(t *testing.T) {
	r := newWasmRegistry()
	reg, ok := r.GetBuilder(wasm.Builder)
	if !ok {
		t.Fatalf("wasm builder not registered")
	}

	wasiRuntimes := []string{
		wasm.RuntimeRustWasi, wasm.RuntimeGoWasi, wasm.RuntimePythonWasi,
		wasm.RuntimeJsWasi, wasm.RuntimeCWasi, wasm.RuntimeCppWasi,
		wasm.RuntimeDotnetWasi, wasm.RuntimeSwiftWasi,
	}
	for _, rt := range wasiRuntimes {
		if !reg.SupportsRuntime(rt) {
			t.Errorf("wasm builder should support WASI runtime %q", rt)
		}
	}

	traditionalRuntimes := []string{"go", "node", "python", "rust", "quarkus"}
	for _, rt := range traditionalRuntimes {
		if reg.SupportsRuntime(rt) {
			t.Errorf("wasm builder should NOT support traditional runtime %q", rt)
		}
	}
}

// TestWasmDeployerAcceptsOnlyWasi verifies that the wasm deployer registration
// only supports WASI runtimes.
func TestWasmDeployerAcceptsOnlyWasi(t *testing.T) {
	r := newWasmRegistry()
	reg, ok := r.GetDeployer(wasm.Deployer)
	if !ok {
		t.Fatalf("wasm deployer not registered")
	}

	wasiRuntimes := []string{
		wasm.RuntimeRustWasi, wasm.RuntimeGoWasi, wasm.RuntimePythonWasi,
	}
	for _, rt := range wasiRuntimes {
		if !reg.SupportsRuntime(rt) {
			t.Errorf("wasm deployer should support WASI runtime %q", rt)
		}
	}

	traditionalRuntimes := []string{"go", "node", "python"}
	for _, rt := range traditionalRuntimes {
		if reg.SupportsRuntime(rt) {
			t.Errorf("wasm deployer should NOT support traditional runtime %q", rt)
		}
	}
}

// TestPostProcessors_TraditionalBuilderRejectsWasi verifies that after
// wasm.Register() installs its post-processors, a traditional builder cannot
// be resolved for a WASI runtime.
func TestPostProcessors_TraditionalBuilderRejectsWasi(t *testing.T) {
	// Add a dummy "pack" builder with no constraints (accepts everything).
	r := fn.NewRegistry()
	r.RegisterBuilder("pack", func(_ fn.BuilderConfig) []fn.Option { return nil })
	wasm.Register(r) // installs post-processors

	reg, ok := r.GetBuilder("pack")
	if !ok {
		t.Fatalf("pack builder not registered")
	}

	// After post-processor, pack should reject WASI runtimes.
	if reg.SupportsRuntime(wasm.RuntimeRustWasi) {
		t.Errorf("pack builder should NOT support WASI runtime after post-processor")
	}
	// But pack should still accept traditional runtimes.
	if !reg.SupportsRuntime("go") {
		t.Errorf("pack builder should still support traditional runtime 'go'")
	}
}

// TestPostProcessors_TraditionalDeployerRejectsWasi verifies the same for deployers.
func TestPostProcessors_TraditionalDeployerRejectsWasi(t *testing.T) {
	r := fn.NewRegistry()
	r.RegisterDeployer("knative", func(_ fn.DeployerConfig) []fn.Option { return nil })
	wasm.Register(r) // installs post-processors

	reg, ok := r.GetDeployer("knative")
	if !ok {
		t.Fatalf("knative deployer not registered")
	}

	if reg.SupportsRuntime(wasm.RuntimeRustWasi) {
		t.Errorf("knative deployer should NOT support WASI runtime after post-processor")
	}
	if !reg.SupportsRuntime("go") {
		t.Errorf("knative deployer should still support traditional runtime 'go'")
	}
}
