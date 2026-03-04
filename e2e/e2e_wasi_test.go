//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"knative.dev/func/pkg/wasm"
)

// TestWasm_RustBuild verifies that a rust-wasi function can be built end-to-end.
// The "wasm" builder is inferred automatically from the rust-wasi runtime; no
// --builder flag is required.  The builder compiles via cargo and pushes a raw
// WASM OCI artifact (not a container image) to the registry.
//
// Prerequisites (skipped automatically when absent):
//   - cargo must be on PATH
//   - wasm32-wasip2 rustup target must be installed: rustup target add wasm32-wasip2
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_RustBuild(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not found on PATH; skipping rust-wasi build test")
	}
	// Skip if the wasm32-wasip2 rustup target is not installed.
	if out, err := exec.Command("rustup", "target", "list", "--installed").Output(); err != nil {
		t.Logf("warn: cannot query rustup targets: %v", err)
	} else if !strings.Contains(string(out), "wasm32-wasip2") {
		t.Skip("wasm32-wasip2 rustup target not installed; run: rustup target add wasm32-wasip2")
	}

	name := "func-e2e-wasm-rust-build"
	_ = fromCleanEnv(t, name)

	// Initialize a rust-wasi function; builder is inferred from runtime.
	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	// Build without --builder; the wasm builder must be auto-selected for WASI runtimes.
	if err := newCmd(t, "build", "--verbose").Run(); err != nil {
		t.Fatalf("func build failed for rust-wasi: %v", err)
	}

	// Second build must also succeed — verifies that the inferred builder "wasm"
	// is NOT written to func.yaml (which would set BuilderExplicit=true on the
	// next invocation and break inference by locking in "wasm" explicitly).
	if err := newCmd(t, "build").Run(); err != nil {
		t.Fatalf("second func build failed for rust-wasi: %v", err)
	}
}

// TestWasm_GoBuild verifies that a go-wasi function can be built end-to-end.
// The "wasm" builder is inferred automatically from the go-wasi runtime; no
// --builder flag is required.  The builder compiles via tinygo and pushes a
// raw WASM OCI artifact to the registry.
//
// Prerequisites (skipped automatically when absent):
//   - tinygo must be on PATH (install from https://tinygo.org)
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_GoBuild(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH; skipping go-wasi build test")
	}
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		t.Skip("wasm-opt not found on PATH (install binaryen https://github.com/WebAssembly/binaryen); skipping go-wasi build test")
	}
	if _, err := exec.LookPath("wasm-tools"); err != nil {
		t.Skip("wasm-tools not found on PATH (install from https://github.com/bytecodealliance/wasm-tools); skipping go-wasi build test")
	}
	// Check that tinygo has the WIT files needed for wasip2 (not always present in distro packages).
	// The official tinygo release from https://tinygo.org includes them; distro packages may not.
	tinygoOut, _ := exec.Command("tinygo", "env", "TINYGOROOT").Output()
	if tinygoOut != nil {
		witDir := strings.TrimSpace(string(tinygoOut)) + "/lib/wasi-cli/wit"
		if _, err := os.Stat(witDir); err != nil {
			t.Skipf("tinygo WIT files not found at %s; install tinygo from https://tinygo.org for wasip2 support", witDir)
		}
	}

	name := "func-e2e-wasm-go-build"
	_ = fromCleanEnv(t, name)

	// Initialize a go-wasi function; builder is inferred from runtime.
	if err := newCmd(t, "init", "-l", wasm.RuntimeGoWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	// Build without --builder; the wasm builder must be auto-selected for WASI runtimes.
	if err := newCmd(t, "build", "--verbose").Run(); err != nil {
		t.Fatalf("func build failed for go-wasi: %v", err)
	}

	// Second build must also succeed — verifies that the inferred builder "wasm"
	// is NOT written to func.yaml (which would set BuilderExplicit=true on the
	// next invocation and break inference by locking in "wasm" explicitly).
	if err := newCmd(t, "build").Run(); err != nil {
		t.Fatalf("second func build failed for go-wasi: %v", err)
	}
}
