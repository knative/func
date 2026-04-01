package wasm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

	// Run go generate if any .go file in the tree contains a //go:generate directive.
	// This is independent of whether a wit/ directory exists.
	hasGenerate, err := hasGoGenerateDirective(ctx, root)
	if err != nil {
		return "", fmt.Errorf("scanning for //go:generate directives: %w", err)
	}
	if hasGenerate {
		// Ensure go.sum is up-to-date before running go generate. Templates
		// ship go.mod with tool directives but no go.sum — go generate would
		// fail trying to run tools without resolved dependencies.
		if err = runGoModTidy(ctx, root, b.verbose); err != nil {
			return "", err
		}
		if err = runGoGenerate(ctx, root, b.verbose); err != nil {
			return "", err
		}
	}

	// Output path for the WASM binary.
	wasmPath = filepath.Join(root, "module.wasm")

	// Build tinygo args.
	// -no-debug strips DWARF debug info, mirroring Rust's strip="symbols" and
	// reducing output size by ~100-200 KB without affecting runtime behaviour.
	args := []string{"build", "-target=wasip2", "-no-debug"}

	// Add WIT flags only when a wit/ directory is present.
	// This allows the same builder to handle functions with and without WIT bindings.
	witDir := filepath.Join(root, "wit")
	if info, statErr := os.Stat(witDir); statErr == nil && info.IsDir() {
		args = append(args, "-wit-package", "wit/", "-wit-world", "boson")
		if b.verbose {
			fmt.Fprintf(os.Stderr, "WIT directory found; adding -wit-package wit/ -wit-world boson\n")
		}
	}

	args = append(args, "-o", "module.wasm", ".")

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

// hasGoGenerateDirective reports whether any .go file in the directory tree
// rooted at root contains a //go:generate directive.
//
// The scan is parallel: each .go file is read in its own goroutine. Scanning
// stops as soon as the first directive is found. The context is used to cancel
// remaining goroutines once a result is determined.
func hasGoGenerateDirective(ctx context.Context, root string) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// found is set to 1 atomically by the first goroutine that finds a directive.
	var found atomic.Int32
	// wg tracks all launched goroutines so we can wait for them to finish.
	var wg sync.WaitGroup
	// sem limits concurrency to avoid overwhelming the OS with open file handles.
	const maxConcurrent = 32
	sem := make(chan struct{}, maxConcurrent)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Stop walking once a directive has been found.
		if found.Load() == 1 {
			return filepath.SkipAll
		}
		// Skip non-.go files (test files CAN contain generate directives;
		// keep them in scope to match standard go generate semantics).
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		filePath := path

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore slot, respecting cancellation.
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			if found.Load() == 1 {
				return
			}

			if fileHasGoGenerate(filePath) {
				found.Store(1)
				cancel() // signal all peers to stop
			}
		}()
		return nil
	})

	wg.Wait()

	if err != nil {
		// filepath.SkipAll causes WalkDir to return nil, so a non-nil err here
		// is a genuine I/O error from the walk itself.
		return false, err
	}

	return found.Load() == 1, nil
}

// fileHasGoGenerate returns true if the file at path contains at least one
// line beginning with "//go:generate" (with no leading whitespace, as per go
// generate specification).
func fileHasGoGenerate(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if bytes.HasPrefix(scanner.Bytes(), []byte("//go:generate")) {
			return true
		}
	}
	return false
}

// runGoModTidy runs "go mod tidy" in root, ensuring go.sum is up-to-date.
// Templates ship go.mod with tool directives but no go.sum — without this
// step, go generate would fail trying to resolve tool dependencies.
func runGoModTidy(ctx context.Context, root string, verbose bool) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go not found on PATH: %w", ErrToolchainNotFound)
	}

	args := []string{"mod", "tidy"}
	cmd := exec.CommandContext(ctx, goPath, args...)
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if verbose {
		fmt.Fprintf(os.Stderr, "cd %s && go %s\n", root, strings.Join(args, " "))
		cmd.Stdout = os.Stderr
	}

	if err = cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}
	return nil
}

// runGoGenerate runs "go generate ./..." in root, streaming output to stderr
// when verbose is true.
func runGoGenerate(ctx context.Context, root string, verbose bool) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go not found on PATH: %w", ErrToolchainNotFound)
	}

	args := []string{"generate", "./..."}
	cmd := exec.CommandContext(ctx, goPath, args...)
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if verbose {
		fmt.Fprintf(os.Stderr, "cd %s && go %s\n", root, strings.Join(args, " "))
		cmd.Stdout = os.Stderr
	}

	if err = cmd.Run(); err != nil {
		return fmt.Errorf("go generate failed: %w", err)
	}
	return nil
}
