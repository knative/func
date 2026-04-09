package wasm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// rustBuilder compiles a Rust WASI function using cargo.
type rustBuilder struct {
	verbose bool
}

// build compiles the Rust source in the given directory to a WASM binary.
// It returns the path to the produced .wasm file.
//
// Prerequisites:
//   - cargo must be on PATH (install via rustup.rs)
//   - wasm32-wasip2 target must be installed: rustup target add wasm32-wasip2
//
// Build implements the Compiler interface.
func (b rustBuilder) Build(ctx context.Context, root string) (wasmPath string, err error) {
	// Verify cargo is available.
	cargoPath, err := exec.LookPath("cargo")
	if err != nil {
		return "", fmt.Errorf("cargo not found on PATH (install Rust from https://rustup.rs): %w", ErrToolchainNotFound)
	}
	if b.verbose {
		fmt.Fprintf(os.Stderr, "Using cargo: %s\n", cargoPath)
	}

	// Verify wasm32-wasip2 target is installed.
	if err = b.checkWasm32Target(ctx, root); err != nil {
		return "", err
	}

	// Run: cargo build --target wasm32-wasip2 --release
	args := []string{"build", "--target", "wasm32-wasip2", "--release"}
	cmd := exec.CommandContext(ctx, cargoPath, args...)
	cmd.Dir = root
	if b.verbose {
		fmt.Fprintf(os.Stderr, "cd %s && cargo %s\n", root, strings.Join(args, " "))
		cmd.Stdout = os.Stderr
	}
	cmd.Stderr = os.Stderr

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("cargo build failed: %w", err)
	}

	// Find the produced .wasm file in target/wasm32-wasip2/release/.
	wasmPath, err = findWasmBinary(root)
	if err != nil {
		return "", err
	}

	if b.verbose {
		fmt.Fprintf(os.Stderr, "Built WASM binary: %s\n", wasmPath)
	}
	return wasmPath, nil
}

// checkWasm32Target verifies the wasm32-wasip2 target is installed.
func (b rustBuilder) checkWasm32Target(ctx context.Context, root string) error {
	cmd := exec.CommandContext(ctx, "rustup", "target", "list", "--installed")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// rustup may not be available; skip the check and let cargo fail naturally.
		if b.verbose {
			fmt.Fprintf(os.Stderr, "WARN: cannot check rustup targets: %v\n", err)
		}
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "wasm32-wasip2" {
			return nil
		}
	}
	return fmt.Errorf("wasm32-wasip2 target not installed; run: rustup target add wasm32-wasip2")
}

// findWasmBinary globs for .wasm files in target/wasm32-wasip2/release/,
// excluding those inside the deps/ subdirectory.
func findWasmBinary(root string) (string, error) {
	releaseDir := filepath.Join(root, "target", "wasm32-wasip2", "release")
	pattern := filepath.Join(releaseDir, "*.wasm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("globbing for wasm binaries: %w", err)
	}

	// Filter out anything inside deps/.
	var candidates []string
	for _, m := range matches {
		rel, err := filepath.Rel(releaseDir, m)
		if err != nil {
			continue
		}
		// Exclude files in deps/ subdirectory.
		if !strings.HasPrefix(rel, "deps"+string(filepath.Separator)) {
			candidates = append(candidates, m)
		}
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no .wasm binary found in %s after cargo build: %w", releaseDir, ErrNoBinaryProduced)
	case 1:
		return candidates[0], nil
	default:
		// Multiple binaries — pick the largest (most likely the real module).
		return largestFile(candidates)
	}
}

// largestFile returns the path of the file with the largest size among the given paths.
func largestFile(paths []string) (string, error) {
	var largest string
	var largestSize int64
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.Size() > largestSize {
			largestSize = info.Size()
			largest = p
		}
	}
	if largest == "" {
		return "", fmt.Errorf("could not determine largest wasm binary among %v", paths)
	}
	return largest, nil
}
