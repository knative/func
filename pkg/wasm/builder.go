package wasm

import (
	"context"
	"fmt"
	"os"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

// Compiler compiles source code to a WASM binary and returns the path to the
// produced .wasm file.
type Compiler interface {
	Build(ctx context.Context, root string) (wasmPath string, err error)
}

// Builder implements fn.Builder for WASI/WASM functions.
// It compiles source code to a WASM binary only; it does NOT push to a registry.
//
// The standard build/push/deploy pipeline is:
//
//	func build  → Builder.Build()  (compile .wasm binary at its natural output location)
//	func build --push or func deploy → Pusher.Push()  (push OCI artifact to registry)
//	func deploy → Deployer.Deploy() (create/update WasmModule CR)
//
// Builder selects a Compiler implementation based on the function runtime
// (rustBuilder for rust-wasi, goBuilder for go-wasi).
// The Compiler can be overridden via WithCompiler for testing or custom integration.
type Builder struct {
	verbose bool

	// compiler is injectable for testing; nil means use the runtime-selected default.
	compiler Compiler
}

// Option is a functional option for Builder.
type Option func(*Builder)

// WithVerbose enables verbose logging.
func WithVerbose(v bool) Option {
	return func(b *Builder) {
		b.verbose = v
	}
}

// WithCompiler overrides the default language-specific Compiler used during
// Build. Primarily intended for testing.
func WithCompiler(c Compiler) Option {
	return func(b *Builder) {
		b.compiler = c
	}
}

// NewBuilder creates a new WASM builder with the given options.
func NewBuilder(opts ...Option) *Builder {
	b := &Builder{}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Build compiles the function at f.Root to a WASM binary.
// The binary is left at its natural compiler output location:
//   - rust-wasi: target/wasm32-wasip2/release/<name>.wasm
//   - go-wasi:   module.wasm (at function root)
//
// Downstream tasks (Pusher, Deployer) locate the binary via WasmBinaryPath.
//
// Build implements the fn.Builder interface.
// Platforms are intentionally ignored for WASM builds — WASM binaries are
// architecture-independent by design.
func (b *Builder) Build(ctx context.Context, f fn.Function, _ []fn.Platform) error {
	if b.verbose {
		fmt.Fprintf(os.Stderr, "Building WASM function %q (runtime: %s)\n", f.Name, f.Runtime)
	}

	// Validate the OCI image reference before doing any work.
	if f.Build.Image == "" {
		return fmt.Errorf("function %q: %w", f.Name, ErrNoImageRef)
	}

	// Select the compiler: use the injected one (for testing) or pick by runtime.
	compiler := b.compiler
	if compiler == nil {
		lang, err := baseLanguage(f.Runtime)
		if err != nil {
			return err
		}
		switch lang {
		case "rust":
			compiler = rustBuilder{verbose: b.verbose}
		case "go":
			compiler = goBuilder{verbose: b.verbose}
		default:
			return fmt.Errorf("WASM builder: runtime %q: %w", f.Runtime, ErrNotImplemented)
		}
	}

	// Compile the source to a .wasm binary.
	wasmPath, err := compiler.Build(ctx, f.Root)
	if err != nil {
		return fmt.Errorf("compiling WASM for runtime %q: %w", f.Runtime, err)
	}

	if b.verbose {
		fmt.Fprintf(os.Stderr, "WASM binary compiled: %s\n", wasmPath)
	}
	return nil
}

// baseLanguage extracts the base language from a WASI runtime identifier.
// For example, "rust-wasi" → "rust", "go-wasi" → "go".
// Returns ErrNotWasiRuntime for non-WASI runtimes.
func baseLanguage(runtime string) (string, error) {
	if !strings.HasSuffix(runtime, WasiSuffix) {
		return "", fmt.Errorf("runtime %q: %w", runtime, ErrNotWasiRuntime)
	}
	lang := strings.TrimSuffix(runtime, WasiSuffix)
	if lang == "" {
		return "", fmt.Errorf("runtime %q: %w", runtime, ErrNotWasiRuntime)
	}
	return lang, nil
}

// WasmBinaryPath returns the path to the compiled WASM binary for function f.
// The path is determined by the runtime (from func.yaml) and the function root
// directory.  This is the canonical location where each compiler leaves its
// output so that the Pusher and other downstream tasks can pick it up without
// any file movement.
//
//   - rust-wasi: target/wasm32-wasip2/release/<name>.wasm  (found via glob)
//   - go-wasi:   module.wasm  (at function root)
func WasmBinaryPath(f fn.Function) (string, error) {
	lang, err := baseLanguage(f.Runtime)
	if err != nil {
		return "", err
	}
	switch lang {
	case "rust":
		return findWasmBinary(f.Root)
	case "go":
		return goWasmBinaryPath(f.Root), nil
	default:
		return "", fmt.Errorf("WASM binary path: runtime %q: %w", f.Runtime, ErrNotImplemented)
	}
}
