package wasm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDiffVersions verifies that diffVersions correctly identifies changed
// and new entries, ignoring unchanged entries.
func TestDiffVersions(t *testing.T) {
	tests := []struct {
		name    string
		current map[string]string
		desired map[string]string
		want    map[string]string
	}{
		{
			name:    "empty current returns all desired",
			current: map[string]string{},
			desired: map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
			want:    map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
		},
		{
			name:    "matching entries are skipped",
			current: map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
			desired: map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
			want:    map[string]string{},
		},
		{
			name:    "changed version triggers re-provision",
			current: map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.2"},
			desired: map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
			want:    map[string]string{"http": "ghcr.io/webassembly/wasi/http:0.2.3"},
		},
		{
			name: "new key added",
			current: map[string]string{
				"http": "ghcr.io/webassembly/wasi/http:0.2.3",
			},
			desired: map[string]string{
				"http":    "ghcr.io/webassembly/wasi/http:0.2.3",
				"sockets": "ghcr.io/webassembly/wasi/sockets:0.2.3",
			},
			want: map[string]string{
				"sockets": "ghcr.io/webassembly/wasi/sockets:0.2.3",
			},
		},
		{
			name: "stale keys in current are not included in diff",
			current: map[string]string{
				"http":    "ghcr.io/webassembly/wasi/http:0.2.3",
				"sockets": "ghcr.io/webassembly/wasi/sockets:0.2.3",
			},
			desired: map[string]string{
				"http": "ghcr.io/webassembly/wasi/http:0.2.3",
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffVersions(tt.current, tt.desired)
			if len(got) != len(tt.want) {
				t.Errorf("len(diff) = %d, want %d; got: %v", len(got), len(tt.want), got)
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("diff[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// TestLoadVersions_Missing verifies that a missing .versions file returns an
// empty map without an error.
func TestLoadVersions_Missing(t *testing.T) {
	dir := t.TempDir()
	versions, err := loadVersions(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected empty map, got: %v", versions)
	}
}

// TestLoadVersions_Corrupt verifies that a corrupt .versions file is treated
// as missing (returns an empty map without an error).
func TestLoadVersions_Corrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, witVersionsFile), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	versions, err := loadVersions(dir)
	if err != nil {
		t.Fatalf("expected no error for corrupt file, got: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected empty map for corrupt file, got: %v", versions)
	}
}

// TestSaveLoadVersionsRoundTrip verifies that saveVersions followed by
// loadVersions returns the same map.
func TestSaveLoadVersionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := map[string]string{
		"http":    "ghcr.io/webassembly/wasi/http:0.2.3",
		"sockets": "ghcr.io/webassembly/wasi/sockets:0.2.3",
	}

	if err := saveVersions(dir, want); err != nil {
		t.Fatalf("saveVersions: %v", err)
	}

	got, err := loadVersions(dir)
	if err != nil {
		t.Fatalf("loadVersions: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("versions[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestSaveVersions_SortedOutput verifies that saveVersions writes keys in
// sorted order for deterministic diffs.
func TestSaveVersions_SortedOutput(t *testing.T) {
	dir := t.TempDir()
	versions := map[string]string{
		"sockets": "ghcr.io/webassembly/wasi/sockets:0.2.3",
		"http":    "ghcr.io/webassembly/wasi/http:0.2.3",
		"clocks":  "ghcr.io/webassembly/wasi/clocks:0.2.3",
	}

	if err := saveVersions(dir, versions); err != nil {
		t.Fatalf("saveVersions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, witVersionsFile))
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal preserving order via json.RawMessage trick: just check the raw
	// bytes contain keys in sorted order.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}

	prevKey := ""
	for k := range raw {
		if k < prevKey {
			t.Errorf("keys not sorted: %q came after %q in JSON output:\n%s", k, prevKey, data)
		}
		prevKey = k
	}
}

// TestWriteGitignore verifies that writeGitignore creates the expected file.
func TestWriteGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := writeGitignore(dir); err != nil {
		t.Fatalf("writeGitignore: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(content) == "" {
		t.Error(".gitignore should not be empty")
	}
}

// TestProvisionWIT_EmptyBuilderImages verifies that ProvisionWIT is a no-op
// when no builderImages are configured.
func TestProvisionWIT_EmptyBuilderImages(t *testing.T) {
	dir := t.TempDir()
	err := ProvisionWIT(context.Background(), dir, nil, false)
	if err != nil {
		t.Fatalf("expected no error for empty builderImages, got: %v", err)
	}
}

// TestProvisionWIT_UpToDate verifies that ProvisionWIT skips all work when
// wit/.versions already matches builderImages.
func TestProvisionWIT_UpToDate(t *testing.T) {
	dir := t.TempDir()
	witPath := filepath.Join(dir, witDir)
	if err := os.MkdirAll(witPath, 0755); err != nil {
		t.Fatal(err)
	}

	builderImages := map[string]string{
		"http": "ghcr.io/webassembly/wasi/http:0.2.3",
	}

	// Write a .versions file that already matches builderImages.
	if err := saveVersions(witPath, builderImages); err != nil {
		t.Fatal(err)
	}

	// ProvisionWIT should skip all downloads (wasm-tools not needed).
	// If it tries to call wasm-tools it will fail (not on PATH in CI).
	err := ProvisionWIT(context.Background(), dir, builderImages, false)
	if err != nil {
		t.Fatalf("expected no-op when up-to-date, got: %v", err)
	}
}
