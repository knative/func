package buildpacks

// LEGACY PYTHON: pack legacy-scaffold tests.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestScaffoldRejectsUnsupportedLegacyPython verifies a pre-v1.18 non-parliament
// Procfile layout (old flask/wsgi templates) is rejected at the scaffold seam.
func TestScaffoldRejectsUnsupportedLegacyPython(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "Procfile"), "web: gunicorn func:main --bind=0.0.0.0:8080\n")
	mustWrite(t, filepath.Join(root, "func.py"), "def main(environ, start_response):\n    ...\n")
	f := fn.Function{Root: root, Runtime: "python"}

	err := NewScaffolder(false).Scaffold(context.Background(), f, "")
	if !errors.Is(err, fn.ErrUnsupportedLegacyPython) {
		t.Fatalf("expected ErrUnsupportedLegacyPython, got %v", err)
	}
}

// TestLegacyScaffold verifies the pack legacy path: it writes a cloudevents<2
// constraints file at the function root, clears any stale modern scaffolding,
// and leaves the user's own files untouched.
func TestLegacyScaffold(t *testing.T) {
	root := t.TempDir()
	// user's own files (must survive)
	mustWrite(t, filepath.Join(root, "Procfile"), "web: python -m parliament .\n")
	mustWrite(t, filepath.Join(root, "requirements.txt"), "parliament-functions==0.1.0\n")
	// stale modern scaffolding from a prior non-legacy build (must be cleared)
	stale := filepath.Join(root, fn.RunDataDir, fn.BuildDir, "service")
	if err := os.MkdirAll(stale, 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(stale, "main.py"), "# stale\n")

	f := fn.Function{Root: root, Runtime: "python"}
	if err := legacyScaffold(false, f); err != nil {
		t.Fatalf("legacyScaffold: %v", err)
	}

	// constraints.txt written with the cloudevents pin
	got, err := os.ReadFile(filepath.Join(root, legacyConstraintsFile))
	if err != nil {
		t.Fatalf("constraints file not written: %v", err)
	}
	if !strings.Contains(string(got), legacyCloudeventsPin) {
		t.Errorf("constraints.txt = %q, want it to contain %q", got, legacyCloudeventsPin)
	}
	// stale scaffolding cleared
	if _, err := os.Stat(filepath.Join(root, fn.RunDataDir, fn.BuildDir)); !os.IsNotExist(err) {
		t.Errorf("stale .func/build was not removed (err=%v)", err)
	}
	// user files untouched
	for _, name := range []string{"Procfile", "requirements.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("user file %q was disturbed: %v", name, err)
		}
	}
}

// TestLegacyPackEnv verifies the pip constraint env is injected for pack builds.
func TestLegacyPackEnv(t *testing.T) {
	env := map[string]string{}
	legacyPackEnv(env)
	if env["PIP_CONSTRAINT"] != legacyConstraintsFile {
		t.Errorf("PIP_CONSTRAINT = %q, want %q", env["PIP_CONSTRAINT"], legacyConstraintsFile)
	}
}

// TestWriteCloudeventsConstraint covers the clobber guard: a pre-existing user
// constraints.txt must be preserved, an existing cloudevents constraint left
// untouched (idempotent on rebuild), and the pin appended otherwise.
func TestWriteCloudeventsConstraint(t *testing.T) {
	read := func(t *testing.T, root string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, legacyConstraintsFile))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	t.Run("absent: writes the pin", func(t *testing.T) {
		root := t.TempDir()
		if err := writeCloudeventsConstraint(root); err != nil {
			t.Fatal(err)
		}
		if got := read(t, root); !strings.Contains(got, legacyCloudeventsPin) {
			t.Errorf("got %q, want it to contain %q", got, legacyCloudeventsPin)
		}
	})

	t.Run("preserves user constraints and appends the pin", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, legacyConstraintsFile), "requests==2.31.0")
		if err := writeCloudeventsConstraint(root); err != nil {
			t.Fatal(err)
		}
		got := read(t, root)
		if !strings.Contains(got, "requests==2.31.0") {
			t.Errorf("clobbered user constraint: %q", got)
		}
		if !strings.Contains(got, legacyCloudeventsPin) {
			t.Errorf("pin not appended: %q", got)
		}
	})

	t.Run("no-op when cloudevents already constrained (idempotent)", func(t *testing.T) {
		root := t.TempDir()
		original := "cloudevents==1.11.0\n"
		mustWrite(t, filepath.Join(root, legacyConstraintsFile), original)
		if err := writeCloudeventsConstraint(root); err != nil {
			t.Fatal(err)
		}
		if got := read(t, root); got != original {
			t.Errorf("user cloudevents constraint disturbed: got %q, want %q", got, original)
		}
	})

	t.Run("appends the pin when cloudevents is mentioned but unconstrained", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, legacyConstraintsFile), "# constraints for the cloudevents stack\ncloudevents\n")
		if err := writeCloudeventsConstraint(root); err != nil {
			t.Fatal(err)
		}
		if got := read(t, root); !strings.Contains(got, legacyCloudeventsPin) {
			t.Errorf("pin not appended over an unconstrained mention: %q", got)
		}
	})

	t.Run("no-op on a case-insensitive constrained entry", func(t *testing.T) {
		root := t.TempDir()
		original := "CloudEvents<1.12\n"
		mustWrite(t, filepath.Join(root, legacyConstraintsFile), original)
		if err := writeCloudeventsConstraint(root); err != nil {
			t.Fatal(err)
		}
		if got := read(t, root); got != original {
			t.Errorf("case-insensitive existing constraint disturbed: got %q", got)
		}
	})
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
