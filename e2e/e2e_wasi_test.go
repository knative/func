//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/wasm"
)

// wasmList returns the list of deployed WASM functions in namespace using the
// wasm lister directly (bypasses the CLI).
func wasmList(t *testing.T, namespace string) []fn.ListItem {
	t.Helper()
	client := fn.New(fn.WithListers(wasm.NewLister()))
	list, err := client.List(context.Background(), namespace)
	if err != nil {
		t.Fatalf("wasm list failed: %v", err)
	}
	return list
}

// requireRustWasi skips the test if the rust-wasi toolchain is unavailable.
func requireRustWasi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not found on PATH; skipping rust-wasi test")
	}
	if out, err := exec.Command("rustup", "target", "list", "--installed").Output(); err != nil {
		t.Logf("warn: cannot query rustup targets: %v", err)
	} else if !strings.Contains(string(out), "wasm32-wasip2") {
		t.Skip("wasm32-wasip2 rustup target not installed; run: rustup target add wasm32-wasip2")
	}
}

// requireGoWasi skips the test if the go-wasi toolchain is unavailable.
func requireGoWasi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH; skipping go-wasi test")
	}
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		t.Skip("wasm-opt not found on PATH (install binaryen https://github.com/WebAssembly/binaryen); skipping go-wasi test")
	}
	if _, err := exec.LookPath("wasm-tools"); err != nil {
		t.Skip("wasm-tools not found on PATH (install from https://github.com/bytecodealliance/wasm-tools); skipping go-wasi test")
	}
	tinygoOut, _ := exec.Command("tinygo", "env", "TINYGOROOT").Output()
	if tinygoOut != nil {
		witDir := strings.TrimSpace(string(tinygoOut)) + "/lib/wasi-cli/wit"
		if _, err := os.Stat(witDir); err != nil {
			t.Skipf("tinygo WIT files not found at %s; install tinygo from https://tinygo.org for wasip2 support", witDir)
		}
	}
}

// TestWasm_RustBuild verifies that a rust-wasi function can be built end-to-end.
// The "wasm" builder is inferred automatically from the rust-wasi runtime; no
// --builder flag is required.
//
// Prerequisites (skipped automatically when absent):
//   - cargo must be on PATH
//   - wasm32-wasip2 rustup target must be installed: rustup target add wasm32-wasip2
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_RustBuild(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-build"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

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
// --builder flag is required.
//
// Prerequisites (skipped automatically when absent):
//   - tinygo must be on PATH (install from https://tinygo.org)
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_GoBuild(t *testing.T) {
	requireGoWasi(t)

	name := "func-e2e-wasm-go-build"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeGoWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

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

// TestWasm_RustDeploy verifies the full build→push→deploy cycle for a rust-wasi
// function.  It deploys to the cluster via "func deploy", confirms the WasmModule
// CR is listed, re-deploys to exercise the update path, and cleans up.
//
// Prerequisites (skipped automatically when absent):
//   - cargo must be on PATH
//   - wasm32-wasip2 rustup target must be installed
//   - A reachable registry (FUNC_E2E_REGISTRY)
//   - A Kubernetes cluster with the WasmModule CRD installed
func TestWasm_RustDeploy(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "deploy", "--verbose").Run(); err != nil {
		t.Fatalf("func deploy failed for rust-wasi: %v", err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("deployed rust-wasi function not found in func list")
	}

	// Re-deploy to exercise the update path (WasmModule already exists).
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatalf("second func deploy (update) failed for rust-wasi: %v", err)
	}
}

// TestWasm_GoDeploy verifies the full build→push→deploy cycle for a go-wasi
// function.  It deploys to the cluster via "func deploy", confirms the WasmModule
// CR is listed, re-deploys to exercise the update path, and cleans up.
//
// Prerequisites (skipped automatically when absent):
//   - tinygo + wasm-opt + wasm-tools must be on PATH
//   - A reachable registry (FUNC_E2E_REGISTRY)
//   - A Kubernetes cluster with the WasmModule CRD installed
func TestWasm_GoDeploy(t *testing.T) {
	requireGoWasi(t)

	name := "func-e2e-wasm-go-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeGoWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "deploy", "--verbose").Run(); err != nil {
		t.Fatalf("func deploy failed for go-wasi: %v", err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("deployed go-wasi function not found in func list")
	}

	// Re-deploy to exercise the update path (WasmModule already exists).
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatalf("second func deploy (update) failed for go-wasi: %v", err)
	}
}

// TestWasm_RustDescribe verifies that "func describe" returns meaningful output
// for a deployed rust-wasi function.
//
// Prerequisites: same as TestWasm_RustDeploy.
func TestWasm_RustDescribe(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-describe"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatalf("func deploy failed: %v", err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if err := newCmd(t, "describe").Run(); err != nil {
		t.Fatalf("func describe failed for rust-wasi: %v", err)
	}
}

// TestWasm_RustDelete verifies that "func delete" removes a deployed rust-wasi
// function from the cluster.
//
// Prerequisites: same as TestWasm_RustDeploy.
func TestWasm_RustDelete(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-delete"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatalf("func deploy failed: %v", err)
	}

	if !containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("function not listed after deploy")
	}

	if err := newCmd(t, "delete", name, "--namespace", Namespace).Run(); err != nil {
		t.Fatalf("func delete failed for rust-wasi: %v", err)
	}

	if containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("function still listed after delete")
	}
}
