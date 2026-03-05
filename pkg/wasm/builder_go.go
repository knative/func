package wasm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goBuilder compiles a Go WASI function using TinyGo.
type goBuilder struct {
	verbose bool
}

// goWasmBinaryPath returns the path where the Go WASM binary will be placed
// relative to the function root (module.wasm at root).
func goWasmBinaryPath(root string) string {
	return filepath.Join(root, "module.wasm")
}

// build compiles the Go source in the given directory to a WASM binary using TinyGo.
// It returns the path to the produced module.wasm file.
//
// Prerequisites:
//   - tinygo must be on PATH (https://tinygo.org)
//   - wasm-opt must be on PATH — part of binaryen (https://github.com/WebAssembly/binaryen)
//   - wasm-tools must be on PATH (https://github.com/bytecodealliance/wasm-tools)
//
// tinygo will report clear errors for any missing dependency.
//
// Build implements the Compiler interface.
func (b goBuilder) Build(ctx context.Context, root string) (wasmPath string, err error) {
	// Verify tinygo is available.
	tinygoPath, err := exec.LookPath("tinygo")
	if err != nil {
		return "", fmt.Errorf("tinygo not found on PATH (install from https://tinygo.org): %w", ErrToolchainNotFound)
	}
	if b.verbose {
		fmt.Fprintf(os.Stderr, "Using tinygo: %s\n", tinygoPath)
	}

	// Output path for the WASM binary.
	wasmPath = filepath.Join(root, "module.wasm")

	// Run: tinygo build -target=wasip2 -no-debug -o module.wasm .
	// -no-debug strips DWARF debug info, mirroring Rust's strip="symbols" and
	// reducing output size by ~100-200 KB without affecting runtime behaviour.
	// tinygo will report errors for any missing dependencies (wasm-opt, wasm-tools).
	args := []string{"build", "-target=wasip2", "-no-debug", "-o", wasmPath, "."}
	cmd := exec.CommandContext(ctx, tinygoPath, args...)
	cmd.Dir = root
	if b.verbose {
		fmt.Fprintf(os.Stderr, "cd %s && tinygo %s\n", root, strings.Join(args, " "))
		cmd.Stdout = os.Stderr
	}
	cmd.Stderr = os.Stderr

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("tinygo build failed: %w", err)
	}

	// Verify the output file was created.
	if _, statErr := os.Stat(wasmPath); statErr != nil {
		return "", fmt.Errorf("tinygo build succeeded but output file not found at %s: %w", wasmPath, statErr)
	}

	if b.verbose {
		fmt.Fprintf(os.Stderr, "Built WASM binary: %s\n", wasmPath)
	}
	return wasmPath, nil
}
