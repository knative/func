package wasm_test

import (
	"testing"

	"knative.dev/func/pkg/wasm"
)

func TestIsWasiRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		runtime  string
		expected bool
	}{
		{"rust-wasi", wasm.RuntimeRustWasi, true},
		{"go-wasi", wasm.RuntimeGoWasi, true},
		{"python-wasi", wasm.RuntimePythonWasi, true},
		{"js-wasi", wasm.RuntimeJsWasi, true},
		{"c-wasi", wasm.RuntimeCWasi, true},
		{"cpp-wasi", wasm.RuntimeCppWasi, true},
		{"dotnet-wasi", wasm.RuntimeDotnetWasi, true},
		{"swift-wasi", wasm.RuntimeSwiftWasi, true},
		{"node (not wasi)", "node", false},
		{"python (not wasi)", "python", false},
		{"go (not wasi)", "go", false},
		{"rust (not wasi)", "rust", false},
		{"empty string", "", false},
		{"unknown runtime", "unknown-wasi", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := wasm.IsWasiRuntime(tt.runtime)
			if result != tt.expected {
				t.Errorf("IsWasiRuntime(%q) = %v, want %v", tt.runtime, result, tt.expected)
			}
		})
	}
}
