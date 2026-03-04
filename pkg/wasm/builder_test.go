package wasm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/wasm"
)

// mockCompiler is an injectable Compiler implementation for testing.
type mockCompiler struct {
	// BuildFn is called when Build is invoked.
	BuildFn func(ctx context.Context, root string) (string, error)
	// capturedRoot is the root directory passed to Build.
	capturedRoot string
}

func (m *mockCompiler) Build(ctx context.Context, root string) (string, error) {
	m.capturedRoot = root
	if m.BuildFn != nil {
		return m.BuildFn(ctx, root)
	}
	// Default: succeed and return a fake wasm path.
	return filepath.Join(root, "module.wasm"), nil
}

// mockPusher is an injectable Pusher implementation for testing.
type mockPusher struct {
	// PushFn is called when Push is invoked.
	PushFn func(ctx context.Context, imageRef, wasmPath string) (string, error)
	// captured fields record what was passed to Push.
	capturedImageRef string
	capturedWasmPath string
}

func (m *mockPusher) Push(ctx context.Context, imageRef, wasmPath string) (string, error) {
	m.capturedImageRef = imageRef
	m.capturedWasmPath = wasmPath
	if m.PushFn != nil {
		return m.PushFn(ctx, imageRef, wasmPath)
	}
	// Default: succeed and return a fake digest.
	return "sha256:deadbeef", nil
}

// TestBuilder_UnsupportedRuntime verifies that Build wraps ErrNotImplemented
// for WASI runtimes that are recognized but not yet implemented.
func TestBuilder_UnsupportedRuntime(t *testing.T) {
	t.Parallel()

	unsupported := []string{
		wasm.RuntimePythonWasi,
		wasm.RuntimeJsWasi,
		wasm.RuntimeCWasi,
		wasm.RuntimeCppWasi,
		wasm.RuntimeDotnetWasi,
		wasm.RuntimeSwiftWasi,
	}

	for _, runtime := range unsupported {
		t.Run(runtime, func(t *testing.T) {
			t.Parallel()
			b := wasm.NewBuilder()
			f := fn.Function{
				Root:    t.TempDir(),
				Runtime: runtime,
				Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
			}
			err := b.Build(context.Background(), f, nil)
			if !errors.Is(err, wasm.ErrNotImplemented) {
				t.Fatalf("expected error wrapping wasm.ErrNotImplemented for runtime %q, got: %v", runtime, err)
			}
		})
	}
}

// TestBuilder_NonWasiRuntime verifies that Build wraps ErrNotWasiRuntime when a
// non-WASI runtime (e.g. "go", "node") is passed to the WASM builder.
func TestBuilder_NonWasiRuntime(t *testing.T) {
	t.Parallel()

	for _, runtime := range []string{"go", "node", "python", "rust", ""} {
		t.Run(runtime, func(t *testing.T) {
			t.Parallel()
			b := wasm.NewBuilder()
			f := fn.Function{
				Root:    t.TempDir(),
				Runtime: runtime,
				Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
			}
			err := b.Build(context.Background(), f, nil)
			if !errors.Is(err, wasm.ErrNotWasiRuntime) {
				t.Fatalf("expected error wrapping wasm.ErrNotWasiRuntime for runtime %q, got: %v", runtime, err)
			}
		})
	}
}

// TestBuilder_MissingImageRef verifies that Build wraps ErrNoImageRef when no
// image reference is configured, before any compilation is attempted.
func TestBuilder_MissingImageRef(t *testing.T) {
	t.Parallel()

	b := wasm.NewBuilder()
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeRustWasi,
		// No Build.Image set.
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, wasm.ErrNoImageRef) {
		t.Fatalf("expected error wrapping wasm.ErrNoImageRef, got: %v", err)
	}
}

// TestBuilder_RustMissingCargo verifies that Build wraps ErrToolchainNotFound
// when cargo is not available on PATH.
func TestBuilder_RustMissingCargo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	b := wasm.NewBuilder()
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeRustWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, wasm.ErrToolchainNotFound) {
		t.Fatalf("expected error wrapping wasm.ErrToolchainNotFound, got: %v", err)
	}
}

// TestBuilder_GoMissingTinygo verifies that Build wraps ErrToolchainNotFound
// when tinygo is not available on PATH.
func TestBuilder_GoMissingTinygo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	b := wasm.NewBuilder()
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeGoWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, wasm.ErrToolchainNotFound) {
		t.Fatalf("expected error wrapping wasm.ErrToolchainNotFound, got: %v", err)
	}
}

// TestBuilder_PlatformsIgnored verifies that the platforms argument is ignored
// (WASM binaries are platform-independent). The result should be the same
// regardless of whether platforms are provided.
func TestBuilder_PlatformsIgnored(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	b := wasm.NewBuilder()
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeRustWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, []fn.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm64"},
	})
	if !errors.Is(err, wasm.ErrToolchainNotFound) {
		t.Fatalf("expected error wrapping wasm.ErrToolchainNotFound (platforms ignored), got: %v", err)
	}
}

// TestBuilder_ImplementsInterface verifies at compile time that *wasm.Builder
// satisfies the fn.Builder interface.
func TestBuilder_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var _ fn.Builder = (*wasm.Builder)(nil)
}

// TestBuilder_FullPipeline verifies the complete build pipeline using
// injectable mock Compiler and Pusher — the same approach as buildpacks/s2i
// which inject mock implementations.
//
// This test verifies:
//   - Compiler.Build is called with the correct root directory
//   - Pusher.Push is called with the correct imageRef and wasmPath
//   - Build returns nil on success
func TestBuilder_FullPipeline(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	imageRef := "registry.example.com/ns/fn:latest"
	expectedWasmPath := filepath.Join(root, "target", "module.wasm")

	compiler := &mockCompiler{
		BuildFn: func(ctx context.Context, compileRoot string) (string, error) {
			if compileRoot != root {
				t.Errorf("compiler got root=%q, want %q", compileRoot, root)
			}
			return expectedWasmPath, nil
		},
	}
	pusher := &mockPusher{
		PushFn: func(ctx context.Context, ref, wasmPath string) (string, error) {
			if ref != imageRef {
				t.Errorf("pusher got imageRef=%q, want %q", ref, imageRef)
			}
			if wasmPath != expectedWasmPath {
				t.Errorf("pusher got wasmPath=%q, want %q", wasmPath, expectedWasmPath)
			}
			return "sha256:abc123", nil
		},
	}

	b := wasm.NewBuilder(
		wasm.WithCompiler(compiler),
		wasm.WithPusher(pusher),
	)
	f := fn.Function{
		Root:    root,
		Runtime: wasm.RuntimeRustWasi, // runtime is irrelevant when compiler is injected
		Build:   fn.BuildSpec{Image: imageRef},
	}
	if err := b.Build(context.Background(), f, nil); err != nil {
		t.Fatalf("Build() unexpected error: %v", err)
	}

	if compiler.capturedRoot != root {
		t.Errorf("compiler.capturedRoot=%q, want %q", compiler.capturedRoot, root)
	}
	if pusher.capturedImageRef != imageRef {
		t.Errorf("pusher.capturedImageRef=%q, want %q", pusher.capturedImageRef, imageRef)
	}
	if pusher.capturedWasmPath != expectedWasmPath {
		t.Errorf("pusher.capturedWasmPath=%q, want %q", pusher.capturedWasmPath, expectedWasmPath)
	}
}

// TestBuilder_CompilerError verifies that a compilation error is propagated.
func TestBuilder_CompilerError(t *testing.T) {
	t.Parallel()

	compileErr := errors.New("compile failed")
	compiler := &mockCompiler{
		BuildFn: func(ctx context.Context, root string) (string, error) {
			return "", compileErr
		},
	}

	b := wasm.NewBuilder(wasm.WithCompiler(compiler))
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeRustWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, compileErr) {
		t.Fatalf("expected error wrapping compileErr, got: %v", err)
	}
}

// TestBuilder_PusherError verifies that a push error is propagated.
func TestBuilder_PusherError(t *testing.T) {
	t.Parallel()

	pushErr := errors.New("push failed")
	compiler := &mockCompiler{}
	pusher := &mockPusher{
		PushFn: func(ctx context.Context, imageRef, wasmPath string) (string, error) {
			return "", pushErr
		},
	}

	b := wasm.NewBuilder(wasm.WithCompiler(compiler), wasm.WithPusher(pusher))
	f := fn.Function{
		Root:    t.TempDir(),
		Runtime: wasm.RuntimeRustWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, pushErr) {
		t.Fatalf("expected error wrapping pushErr, got: %v", err)
	}
}

// TestFindWasmBinary_NoWasm verifies that when the build toolchain succeeds
// but produces no .wasm binary, Build wraps ErrNoBinaryProduced.
func TestFindWasmBinary_NoWasm(t *testing.T) {
	fakeDir := t.TempDir()

	// Fake cargo that exits successfully without producing any output.
	fakeCargoPath := filepath.Join(fakeDir, "cargo")
	if err := os.WriteFile(fakeCargoPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// Fake rustup that reports wasm32-wasip2 as installed.
	fakeRustupPath := filepath.Join(fakeDir, "rustup")
	if err := os.WriteFile(fakeRustupPath, []byte("#!/bin/sh\necho wasm32-wasip2\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	// Create the release directory but leave it empty (no .wasm files).
	releaseDir := filepath.Join(root, "target", "wasm32-wasip2", "release")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	b := wasm.NewBuilder()
	f := fn.Function{
		Root:    root,
		Runtime: wasm.RuntimeRustWasi,
		Build:   fn.BuildSpec{Image: "reg/ns/fn:latest"},
	}
	err := b.Build(context.Background(), f, nil)
	if !errors.Is(err, wasm.ErrNoBinaryProduced) {
		t.Fatalf("expected error wrapping wasm.ErrNoBinaryProduced, got: %v", err)
	}
}
