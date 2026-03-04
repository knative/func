package wasm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

// Compiler compiles source code to a WASM binary and returns the path to the
// produced .wasm file.
type Compiler interface {
	Build(ctx context.Context, root string) (wasmPath string, err error)
}

// Pusher pushes a WASM binary as an OCI artifact to the registry at imageRef
// and returns the content digest.
type Pusher interface {
	Push(ctx context.Context, imageRef, wasmPath string) (digest string, err error)
}

// Builder implements fn.Builder for WASI/WASM functions.
// It compiles source code to a WASM binary and pushes it as an OCI artifact.
//
// By default, Builder selects a Compiler implementation based on the function
// runtime (rustBuilder for rust-wasi, goBuilder for go-wasi) and uses the
// default OCI pusher. Both can be overridden via WithCompiler and WithPusher
// for testing or custom integration.
type Builder struct {
	verbose             bool
	credentialsProvider CredentialsProvider
	transport           http.RoundTripper
	insecure            bool

	// compiler and pusher are injectable for testing; nil means use defaults.
	compiler Compiler
	pusher   Pusher
}

// Option is a functional option for Builder.
type Option func(*Builder)

// WithVerbose enables verbose logging.
func WithVerbose(v bool) Option {
	return func(b *Builder) {
		b.verbose = v
	}
}

// WithCredentialsProvider sets the registry credentials provider.
func WithCredentialsProvider(cp CredentialsProvider) Option {
	return func(b *Builder) {
		b.credentialsProvider = cp
	}
}

// WithTransport sets the HTTP transport for registry communication.
func WithTransport(t http.RoundTripper) Option {
	return func(b *Builder) {
		b.transport = t
	}
}

// WithInsecure disables TLS certificate verification for registry communication.
func WithInsecure(insecure bool) Option {
	return func(b *Builder) {
		b.insecure = insecure
	}
}

// WithCompiler overrides the default language-specific Compiler used during
// Build. Primarily intended for testing.
func WithCompiler(c Compiler) Option {
	return func(b *Builder) {
		b.compiler = c
	}
}

// WithPusher overrides the default OCI Pusher used during Build.
// Primarily intended for testing.
func WithPusher(p Pusher) Option {
	return func(b *Builder) {
		b.pusher = p
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

// Build compiles the function at f.Root to a WASM binary, then pushes it
// as an OCI WASM artifact to the registry, and sets f.Build.Image.
//
// Build implements the fn.Builder interface:
//
//	func Build(ctx context.Context, f fn.Function, platforms []fn.Platform) error
//
// Platforms are intentionally ignored for WASM builds — WASM binaries are
// architecture-independent by design.
func (b *Builder) Build(ctx context.Context, f fn.Function, _ []fn.Platform) error {
	if b.verbose {
		fmt.Fprintf(os.Stderr, "Building WASM function %q (runtime: %s)\n", f.Name, f.Runtime)
	}

	// Validate the OCI image reference before doing any work.
	imageRef := f.Build.Image
	if imageRef == "" {
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

	// Select the pusher: use the injected one (for testing) or the real OCI pusher.
	pusher := b.pusher
	if pusher == nil {
		pusher = &ociPusher{
			credentialsProvider: b.credentialsProvider,
			transport:           b.transport,
			insecure:            b.insecure,
			verbose:             b.verbose,
		}
	}

	// Push the WASM binary as an OCI artifact.
	digest, err := pusher.Push(ctx, imageRef, wasmPath)
	if err != nil {
		return fmt.Errorf("pushing WASM OCI artifact: %w", err)
	}

	if b.verbose {
		fmt.Fprintf(os.Stderr, "Pushed WASM OCI artifact: %s@%s\n", imageRef, digest)
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
