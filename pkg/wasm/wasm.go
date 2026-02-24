// Package wasm provides constants, types, and utilities for WASI/WebAssembly
// function support in the func CLI.
package wasm

const (
	// Builder is the name of the WASM builder subsystem
	Builder = "wasm"

	// Deployer is the name of the WASM deployer subsystem
	Deployer = "wasm"

	// MediaType is the OCI media type for WASM modules
	// See: https://github.com/opencontainers/artifacts/blob/main/artifact-authors.md#defining-a-unique-artifact-type
	MediaType = "application/vnd.wasm.module.v1+wasm"

	// WasiSuffix is the suffix used to identify WASI runtimes
	WasiSuffix = "-wasi"
)

// WASI runtime identifiers for supported languages
const (
	RuntimeRustWasi   = "rust-wasi"
	RuntimeGoWasi     = "go-wasi"
	RuntimePythonWasi = "python-wasi"
	RuntimeJsWasi     = "js-wasi"
	RuntimeCWasi      = "c-wasi"
	RuntimeCppWasi    = "cpp-wasi"
	RuntimeDotnetWasi = "dotnet-wasi"
	RuntimeSwiftWasi  = "swift-wasi"
)

// IsWasiRuntime returns true if the given runtime string represents a WASI runtime.
// WASI runtimes are identified by the "-wasi" suffix.
func IsWasiRuntime(runtime string) bool {
	if runtime == "" {
		return false
	}
	// Check for known WASI runtimes
	switch runtime {
	case RuntimeRustWasi, RuntimeGoWasi, RuntimePythonWasi, RuntimeJsWasi,
		RuntimeCWasi, RuntimeCppWasi, RuntimeDotnetWasi, RuntimeSwiftWasi:
		return true
	}
	return false
}

// AllWasiRuntimes returns a slice of all known WASI runtime identifiers.
func AllWasiRuntimes() []string {
	return []string{
		RuntimeRustWasi,
		RuntimeGoWasi,
		RuntimePythonWasi,
		RuntimeJsWasi,
		RuntimeCWasi,
		RuntimeCppWasi,
		RuntimeDotnetWasi,
		RuntimeSwiftWasi,
	}
}
