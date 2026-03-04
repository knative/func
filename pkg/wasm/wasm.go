// Package wasm provides constants, types, and utilities for WASI/WebAssembly
// function support in the func CLI.
package wasm

import "errors"

const (
	// BuilderName is the short name of the WASM builder subsystem.
	BuilderName = "wasm"

	// DeployerName is the short name of the WASM deployer subsystem.
	DeployerName = "wasm"

	// MediaType is the OCI media type for WASM modules
	// See: https://github.com/opencontainers/artifacts/blob/main/artifact-authors.md#defining-a-unique-artifact-type
	MediaType = "application/vnd.wasm.module.v1+wasm"

	// WasiSuffix is the suffix used to identify WASI runtimes
	WasiSuffix = "-wasi"
)

// ErrNotImplemented is returned when a WASI runtime is recognized but its
// build toolchain integration has not been implemented yet.
var ErrNotImplemented = errors.New("not yet implemented")

// ErrNotWasiRuntime is returned when a non-WASI runtime is passed to the
// WASM builder (e.g. "go", "node", "python").
var ErrNotWasiRuntime = errors.New("not a WASI runtime")

// ErrNoImageRef is returned when a function has no image reference configured.
var ErrNoImageRef = errors.New("no image reference configured")

// ErrToolchainNotFound is returned when a required build tool (cargo, tinygo,
// etc.) is not found on the PATH.
var ErrToolchainNotFound = errors.New("build toolchain not found")

// ErrNoBinaryProduced is returned when the build toolchain succeeds but no
// .wasm binary can be located in the expected output directory.
var ErrNoBinaryProduced = errors.New("no WASM binary produced")

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
