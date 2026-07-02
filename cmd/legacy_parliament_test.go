package cmd

// LEGACY PYTHON: func-run host rejection test.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
)

// TestRun_LegacyParliamentHostRejected verifies `func run --builder=host` on a
// legacy parliament function is rejected up front, before any build or run.
func TestRun_LegacyParliamentHostRejected(t *testing.T) {
	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Root: root, Runtime: "python"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Procfile"), []byte("web: python -m parliament .\n"), 0644); err != nil {
		t.Fatal(err)
	}

	builder := mock.NewBuilder()
	runner := mock.NewRunner()
	cmd := NewRunCmd(NewTestClient(
		fn.WithBuilder(builder),
		fn.WithRunner(runner),
		fn.WithRegistry("ghcr.com/reg"),
	))
	cmd.SetArgs([]string{"--builder=host"})

	_, err := cmd.ExecuteContextC(t.Context())
	if !errors.Is(err, oci.ErrLegacyParliamentHost) {
		t.Fatalf("expected ErrLegacyParliamentHost, got %v", err)
	}
	if builder.BuildInvoked {
		t.Error("build should not run for a rejected legacy parliament host run")
	}
	if runner.RunInvoked {
		t.Error("run should not run for a rejected legacy parliament host run")
	}
}

// TestRun_UnsupportedLegacyPythonHostRejected verifies `func run --builder=host`
// on a pre-v1.18 non-parliament Procfile layout (old flask/wsgi templates) is
// rejected up front with the migration error.
func TestRun_UnsupportedLegacyPythonHostRejected(t *testing.T) {
	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Root: root, Runtime: "python"}); err != nil {
		t.Fatal(err)
	}
	// turn the modern scaffold into the old flask/wsgi shape: a gunicorn
	// Procfile at root, no root pyproject.toml.
	if err := os.Remove(filepath.Join(root, "pyproject.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Procfile"), []byte("web: gunicorn func:main --bind=0.0.0.0:8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	builder := mock.NewBuilder()
	runner := mock.NewRunner()
	cmd := NewRunCmd(NewTestClient(
		fn.WithBuilder(builder),
		fn.WithRunner(runner),
		fn.WithRegistry("ghcr.com/reg"),
	))
	cmd.SetArgs([]string{"--builder=host"})

	_, err := cmd.ExecuteContextC(t.Context())
	if !errors.Is(err, fn.ErrUnsupportedLegacyPython) {
		t.Fatalf("expected ErrUnsupportedLegacyPython, got %v", err)
	}
	if builder.BuildInvoked {
		t.Error("build should not run for a rejected legacy python host run")
	}
	if runner.RunInvoked {
		t.Error("run should not run for a rejected legacy python host run")
	}
}
