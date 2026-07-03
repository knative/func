package s2i

// LEGACY PYTHON: s2i legacy-assemble tests.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestScaffoldRejectsUnsupportedLegacyPython verifies a pre-v1.18 non-parliament
// Procfile layout (old flask/wsgi templates) is rejected at the scaffold seam.
func TestScaffoldRejectsUnsupportedLegacyPython(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Procfile"), []byte("web: gunicorn func:application --bind=0.0.0.0:8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := fn.Function{Root: root, Runtime: "python"}

	err := NewScaffolder(false).Scaffold(context.Background(), f, "")
	if !errors.Is(err, fn.ErrUnsupportedLegacyPython) {
		t.Fatalf("expected ErrUnsupportedLegacyPython, got %v", err)
	}
}

// TestLegacyParliamentAssembler checks the assemble is valid bash, pins
// cloudevents<2 after the requirements install, and fails fast without one.
func TestLegacyParliamentAssembler(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	// bash -n: syntax check without executing.
	cmd := exec.Command(bash, "-n")
	cmd.Stdin = strings.NewReader(LegacyParliamentAssembler)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assemble script is not valid bash: %v\n%s", err, out)
	}

	// the cloudevents<2 pin must run after the requirements install
	reqIdx := strings.Index(LegacyParliamentAssembler, "pip install -r requirements.txt")
	pinIdx := strings.Index(LegacyParliamentAssembler, "cloudevents<2")
	if reqIdx < 0 || pinIdx < 0 {
		t.Fatalf("assembler missing requirements install (%d) or cloudevents pin (%d)", reqIdx, pinIdx)
	}
	if pinIdx < reqIdx {
		t.Error("cloudevents<2 pin must come after the requirements install")
	}

	if !strings.Contains(LegacyParliamentAssembler, "missing requirements.txt") {
		t.Error("assembler must fail fast when requirements.txt is absent")
	}
}

// TestWriteLegacyAssemble checks the assemble lands at .func/build/bin/assemble
// (executable), not the root .s2i/bin that WarnIfLegacyS2IScaffolding flags.
func TestWriteLegacyAssemble(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, defaultPath)
	f := fn.Function{Root: root, Runtime: "python"}

	if err := writeLegacyAssemble(false, f, appRoot); err != nil {
		t.Fatalf("writeLegacyAssemble: %v", err)
	}

	assemble := filepath.Join(appRoot, "bin", "assemble")
	info, err := os.Stat(assemble)
	if err != nil {
		t.Fatalf("assemble not written at %s: %v", assemble, err)
	}
	// Windows has no unix permission bits; the 0700 matters where the script
	// actually executes (the linux build container).
	if runtime.GOOS != "windows" && info.Mode().Perm()&0100 == 0 {
		t.Errorf("assemble is not executable: mode %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(root, ".s2i", "bin", "assemble")); !os.IsNotExist(err) {
		t.Errorf("legacy assemble must not be written to root .s2i/bin (would trip WarnIfLegacyS2IScaffolding)")
	}
}

// TestLegacyImageOverride pins python-39 only when the resolved image is still
// the default; an explicit user override is respected.
func TestLegacyImageOverride(t *testing.T) {
	legacy := fn.Function{Root: t.TempDir(), Runtime: "python"}
	mustWriteFile(t, filepath.Join(legacy.Root, "Procfile"), "web: python -m parliament .\n")

	if got := legacyImageOverride(legacy, DefaultPythonBuilder); got != legacyPythonBuilder {
		t.Errorf("default image not pinned: got %q, want %q", got, legacyPythonBuilder)
	}
	const custom = "registry.example.com/my/python:custom"
	if got := legacyImageOverride(legacy, custom); got != custom {
		t.Errorf("user override not respected: got %q, want %q", got, custom)
	}
	// non-legacy function is never pinned
	modern := fn.Function{Root: t.TempDir(), Runtime: "python"}
	if got := legacyImageOverride(modern, DefaultPythonBuilder); got != DefaultPythonBuilder {
		t.Errorf("modern function should keep default: got %q", got)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
