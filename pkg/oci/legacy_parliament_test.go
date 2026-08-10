package oci

// LEGACY PYTHON: host-builder rejection tests.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestLegacyParliamentHostRejected verifies the host builder refuses a legacy
// parliament function up front (rather than attempting to build a broken image).
func TestLegacyParliamentHostRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Procfile"), []byte("web: python -m parliament .\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := fn.Function{Root: root, Runtime: "python"}

	err := NewBuilder("host", false).Build(context.Background(), f, nil)
	if !errors.Is(err, ErrLegacyParliamentHost) {
		t.Fatalf("expected ErrLegacyParliamentHost, got %v", err)
	}
}

// TestNonLegacyPythonNotRejected guards against the host rejection firing on a
// modern python function (which the host builder does support).
func TestNonLegacyPythonNotRejected(t *testing.T) {
	root := t.TempDir()
	// modern layout: no parliament Procfile / import
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname = \"f\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := fn.Function{Root: root, Runtime: "python"}

	err := NewBuilder("host", false).Build(context.Background(), f, nil)
	if errors.Is(err, ErrLegacyParliamentHost) || errors.Is(err, fn.ErrUnsupportedLegacyPython) {
		t.Fatal("modern python function must not be rejected as legacy")
	}
}

// TestUnsupportedLegacyPythonHostRejected verifies the host builder also refuses
// the pre-v1.18 non-parliament Procfile layouts (old flask/wsgi templates), with
// the migration error.
func TestUnsupportedLegacyPythonHostRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Procfile"), []byte("web: gunicorn func:main --bind=0.0.0.0:8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := fn.Function{Root: root, Runtime: "python"}

	err := NewBuilder("host", false).Build(context.Background(), f, nil)
	if !errors.Is(err, fn.ErrUnsupportedLegacyPython) {
		t.Fatalf("expected ErrUnsupportedLegacyPython, got %v", err)
	}
}
