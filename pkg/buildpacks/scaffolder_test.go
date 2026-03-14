package buildpacks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestScaffoldPython_LocalBuild ensures that Scaffold writes scaffolding
// to .func/build/ when path is empty (local pack builds). User files at
// the function root must not be moved or modified.
func TestScaffoldPython_LocalBuild(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	// Create a user file that should NOT be moved
	if err := os.WriteFile(filepath.Join(root, "func.py"), []byte("def handle(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewScaffolder(false)
	f := fn.Function{Root: root, Runtime: "python", Invoke: "http"}

	// Empty path = local build → writes to .func/build/
	if err := s.Scaffold(context.Background(), f, ""); err != nil {
		t.Fatal(err)
	}

	// User files must not be moved
	if _, err := os.Stat(filepath.Join(root, "func.py")); err != nil {
		t.Error("user file func.py should still be at root after local scaffold")
	}
	// Scaffolding files should NOT exist at root
	if _, err := os.Stat(filepath.Join(root, "Procfile")); !os.IsNotExist(err) {
		t.Error("Procfile should not exist at root after local scaffold")
	}
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); !os.IsNotExist(err) {
		t.Error("pyproject.toml should not exist at root after local scaffold")
	}

	// Scaffolding files should exist in .func/build/
	buildDir := filepath.Join(root, ".func", "build")
	for _, name := range []string{"pyproject.toml", "Procfile", "service/main.py", "service/__init__.py"} {
		if _, err := os.Stat(filepath.Join(buildDir, name)); err != nil {
			t.Errorf(".func/build/%s should exist after local scaffold", name)
		}
	}
}

// TestScaffoldPython_ScaffoldingContent verifies the scaffolding output:
// pyproject.toml is patched for Poetry, and Procfile uses module invocation.
func TestScaffoldPython_ScaffoldingContent(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	s := NewScaffolder(false)
	f := fn.Function{Root: root, Runtime: "python", Invoke: "http"}
	outDir := filepath.Join(root, "out")
	if err := s.Scaffold(context.Background(), f, outDir); err != nil {
		t.Fatal(err)
	}

	// pyproject.toml: template patched for pack/Poetry
	pyproject, err := os.ReadFile(filepath.Join(outDir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(pyproject)
	if !strings.Contains(text, "func-python==") {
		t.Error("pyproject.toml should contain pinned func-python dependency")
	}
	if strings.Contains(text, `{root:uri}`) {
		t.Error("pyproject.toml should not contain {root:uri} after pack patching")
	}
	if !strings.Contains(text, `./f`) {
		t.Error("pyproject.toml should use ./f after pack patching")
	}
	if !strings.Contains(text, "[tool.poetry.dependencies]") {
		t.Error("pyproject.toml should contain [tool.poetry.dependencies] for pack builds")
	}

	// Procfile: module invocation
	procfile, err := os.ReadFile(filepath.Join(outDir, "Procfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(procfile), "python -m service.main") {
		t.Errorf("Procfile should use 'python -m service.main', got: %s", procfile)
	}
}

// TestScaffoldGo_WritesToSubdir ensures Go scaffolding writes to .func/build/
// and does not modify the function root.
func TestScaffoldGo_WritesToSubdir(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: "example.com/alice",
	}

	var err error
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	s := NewScaffolder(false)
	if err := s.Scaffold(context.Background(), f, ""); err != nil {
		t.Fatal(err)
	}

	buildDir := filepath.Join(root, ".func", "build")
	if _, err := os.Stat(buildDir); err != nil {
		t.Fatalf(".func/build/ directory should exist: %v", err)
	}

	// Verify scaffolding content exists in the build directory
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error(".func/build/ should not be empty after Go scaffolding")
	}
}

// TestScaffold_UnsupportedRuntime ensures unsupported runtimes are a no-op.
func TestScaffold_UnsupportedRuntime(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	s := NewScaffolder(false)
	f := fn.Function{Root: root, Runtime: "rust"}

	if err := s.Scaffold(context.Background(), f, ""); err != nil {
		t.Fatalf("unsupported runtime should not error, got: %v", err)
	}
}

// TestScaffoldPython_InlineBuildpack verifies the inline buildpack script
// moves scaffolding from .func/build/ to root and preserves keep-list entries.
func TestScaffoldPython_InlineBuildpack(t *testing.T) {
	script := pythonScaffoldScript()

	// Should move scaffolding from .func/build/ to root
	if !strings.Contains(script, "mv .func/build/*") {
		t.Error("inline buildpack should move scaffolding from .func/build/")
	}

	// Should clean up .func after moving
	if !strings.Contains(script, "rm -rf .func") {
		t.Error("inline buildpack should remove .func after moving")
	}

	// Should create f -> fn symlink
	if !strings.Contains(script, "ln -snf fn f") {
		t.Error("inline buildpack should create f -> fn symlink")
	}

	// Should skip infrastructure entries when moving user code into fn/
	for _, entry := range []string{".func", ".git", ".gitignore", "func.yaml"} {
		if !strings.Contains(script, entry) {
			t.Errorf("inline buildpack script should reference %q in keep-list", entry)
		}
	}
}

// TestScaffoldPython_InvalidInvoke ensures that invalid invoke types
// are rejected at scaffolding time (via scaffolding.Write/detectSignature).
func TestScaffoldPython_InvalidInvoke(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	if err := os.WriteFile(filepath.Join(root, "func.yaml"), []byte("name: test"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewScaffolder(false)
	f := fn.Function{Root: root, Runtime: "python", Invoke: "invalid-type"}
	if err := s.Scaffold(context.Background(), f, filepath.Join(root, "out")); err == nil {
		t.Fatal("expected error for invalid invoke type")
	}
}
