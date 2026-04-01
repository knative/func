package wasm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHasGoGenerateDirective_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	found, err := hasGoGenerateDirective(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected false when no //go:generate directive present")
	}
}

func TestHasGoGenerateDirective_Found(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("//go:generate echo hello\npackage main\n"), 0644)

	found, err := hasGoGenerateDirective(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected true when //go:generate directive present")
	}
}

func TestHasGoGenerateDirective_NestedSubdir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub", "deep")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(sub, "gen.go"), []byte("//go:generate echo nested\npackage deep\n"), 0644)

	found, err := hasGoGenerateDirective(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected true when //go:generate directive is in nested subdir")
	}
}

func TestHasGoGenerateDirective_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	found, err := hasGoGenerateDirective(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected false for empty directory")
	}
}

func TestHasGoGenerateDirective_SkipsNonGoFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("//go:generate echo trick\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.txt"), []byte("//go:generate echo trick\n"), 0644)

	found, err := hasGoGenerateDirective(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected false when //go:generate only in non-.go files")
	}
}

// TestHasGoGenerateDirective_StressNoCrash creates many .go files to exercise
// the concurrent goroutine fan-out and ensure no panics (e.g. double-close).
// Run with -race to validate.
func TestHasGoGenerateDirective_StressNoCrash(t *testing.T) {
	dir := t.TempDir()

	// Create 200 files without the directive.
	for i := 0; i < 200; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		os.WriteFile(p, []byte("package main\n"), 0644)
	}

	for i := 0; i < 50; i++ {
		found, err := hasGoGenerateDirective(context.Background(), dir)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if found {
			t.Fatalf("iteration %d: expected false", i)
		}
	}
}

// TestHasGoGenerateDirective_StressWithDirective creates many .go files with
// the directive buried in the middle to exercise early-exit in parallel scan.
func TestHasGoGenerateDirective_StressWithDirective(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 200; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		content := "package main\n"
		if i == 100 {
			content = "//go:generate echo found\npackage main\n"
		}
		os.WriteFile(p, []byte(content), 0644)
	}

	for i := 0; i < 50; i++ {
		found, err := hasGoGenerateDirective(context.Background(), dir)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if !found {
			t.Fatalf("iteration %d: expected true", i)
		}
	}
}

func TestFileHasGoGenerate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty file", "", false},
		{"no directive", "package main\nfunc main() {}\n", false},
		{"directive at start", "//go:generate echo hello\npackage main\n", true},
		{"directive mid-file", "package main\n\n//go:generate echo hello\n", true},
		{"indented (not valid)", "  //go:generate echo hello\n", false},
		{"comment with go:generate text", "// see //go:generate for details\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*.go")
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString(tt.content)
			f.Close()

			got := fileHasGoGenerate(f.Name())
			if got != tt.want {
				t.Errorf("fileHasGoGenerate() = %v, want %v", got, tt.want)
			}
		})
	}
}
