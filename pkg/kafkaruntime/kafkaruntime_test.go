package kafkaruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// embedFS returns an in-memory stand-in for the embedded directory "bin/x"
// holding the given binary content and version stamp.
func embedFS(content, version string) fstest.MapFS {
	return fstest.MapFS{
		"bin/x/.gitkeep":        {Data: []byte("#")},
		"bin/x/VERSION":         {Data: []byte(version + "\n")},
		"bin/x/" + binaryName(): {Data: []byte(content), Mode: 0o644},
	}
}

func TestExtract_Fresh(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "kafka-runtime", Version)

	path, err := extract(embedFS("binary", Version), "bin/x", dst)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dst, binaryName()) {
		t.Fatalf("extracted to %q, want %q", path, filepath.Join(dst, binaryName()))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("extracted content %q, want %q", got, "binary")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("extracted mode %v, want 0755", fi.Mode().Perm())
		}
	}
	// No temporary files are left behind.
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only the binary in %v, found %v", dst, entries)
	}
}

// TestExtract_Cached ensures an intact, previously extracted binary is reused
// rather than rewritten on every run.
func TestExtract_Cached(t *testing.T) {
	dst := t.TempDir()
	fsys := embedFS("binary", Version)

	path, err := extract(fsys, "bin/x", dst)
	if err != nil {
		t.Fatal(err)
	}
	// Make the cached copy read-only: a rewrite would replace it with a new
	// file (rename), changing its mode back to 0755.
	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = extract(fsys, "bin/x", dst); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode() != before.Mode() {
		t.Fatal("expected the cached binary to be reused, but it was rewritten")
	}
}

// TestExtract_Mismatch ensures a cached binary whose content differs from the
// embedded one (corrupted, or truncated by an interrupted write) is replaced.
func TestExtract_Mismatch(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, binaryName()), []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := extract(embedFS("binary", Version), "bin/x", dst)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("expected the mismatched binary to be replaced, got content %q", got)
	}
}

func TestExtract_Unavailable(t *testing.T) {
	// An embedded directory holding only the placeholder, as in a build that
	// skipped hack/kafka-runtime.sh.
	fsys := fstest.MapFS{"bin/x/.gitkeep": {Data: []byte("#")}}
	if _, err := extract(fsys, "bin/x", t.TempDir()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	// A platform without an embedded directory at all.
	if _, err := extract(fstest.MapFS{}, "", t.TempDir()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable for an empty embed, got %v", err)
	}
}

// TestExtract_VersionMismatch ensures a binary built from a different runtime
// version than the pinned one is refused rather than run.
func TestExtract_VersionMismatch(t *testing.T) {
	_, err := extract(embedFS("binary", "v0.0.1"), "bin/x", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "v0.0.1") || !strings.Contains(err.Error(), Version) {
		t.Fatalf("expected a version mismatch error naming both versions, got %v", err)
	}

	fsys := embedFS("binary", Version)
	delete(fsys, "bin/x/VERSION")
	if _, err := extract(fsys, "bin/x", t.TempDir()); err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected an error for a missing version stamp, got %v", err)
	}
}

func TestExtract_NoDestination(t *testing.T) {
	if _, err := extract(embedFS("binary", Version), "bin/x", ""); err == nil {
		t.Fatal("expected an error when no destination directory is available")
	}
}

func TestBinary_Override(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "my-runtime")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(BinaryEnv, bin)

	got, err := Binary()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("Binary() = %q, want the override %q", got, bin)
	}

	t.Setenv(BinaryEnv, filepath.Join(t.TempDir(), "missing"))
	if _, err := Binary(); err == nil || !strings.Contains(err.Error(), BinaryEnv) {
		t.Fatalf("expected an error naming %v for a missing override, got %v", BinaryEnv, err)
	}
}

func TestDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if want := filepath.Join(xdg, "func", "kafka-runtime", Version); Dir() != want {
		t.Fatalf("Dir() = %q, want %q", Dir(), want)
	}
}

// TestEmbedDirs ensures every bin/<goos>_<goarch> directory has the matching
// embed file and vice versa, so hack/kafka-runtime.sh (which builds for any
// platform with a bin directory) and the build-tagged embeds stay in step.
func TestEmbedDirs(t *testing.T) {
	dirs, err := os.ReadDir("bin")
	if err != nil {
		t.Fatal(err)
	}
	platforms := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		platforms[d.Name()] = true
		if _, err := os.Stat("embed_" + d.Name() + ".go"); err != nil {
			t.Errorf("bin/%v has no matching embed_%v.go", d.Name(), d.Name())
		}
	}
	files, err := filepath.Glob("embed_*_*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		p := strings.TrimSuffix(strings.TrimPrefix(f, "embed_"), ".go")
		if !platforms[p] {
			t.Errorf("%v has no matching bin/%v directory", f, p)
		}
	}
	if len(platforms) == 0 {
		t.Fatal("no embedded platforms found")
	}
}
