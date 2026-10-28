//go:build linux

package main

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/source-to-image/pkg/scripts"
)

// TestResolveScriptsURL ensures s2i reads the scaffolded scripts from the
// function when the function is in a subdirectory of the working directory,
// as it is for a remote build of a function in a subdirectory of its git
// repository.
func TestResolveScriptsURL(t *testing.T) {
	root := t.TempDir()
	funcRoot := filepath.Join(root, "fn")
	bin := filepath.Join(funcRoot, ".func", "build", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "assemble"), []byte("scaffolded"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	resolved, err := resolveScriptsURL("file://.func/build/bin", funcRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Read the script as s2i does: URLScriptHandler builds its URL and
	// FileURLReader opens it.
	r := (&scripts.URLScriptHandler{URL: resolved}).Get("assemble")
	if r == nil {
		t.Fatalf("s2i could not build a script URL from %q", resolved)
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := (&scripts.FileURLReader{}).Read(u)
	if err != nil {
		t.Fatalf("s2i cannot read the scaffolded assemble from %q: %v", resolved, err)
	}
	defer rc.Close()
	bs, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(bs) != "scaffolded" {
		t.Errorf("read %q, want the scaffolded assemble", bs)
	}

	for _, raw := range []string{
		"image:///usr/libexec/s2i",
		"file:///usr/local/s2i",
		"https://example.com/s2i",
	} {
		got, err := resolveScriptsURL(raw, funcRoot)
		if err != nil {
			t.Fatal(err)
		}
		if got != raw {
			t.Errorf("resolveScriptsURL(%q) = %q, want it unchanged", raw, got)
		}
	}
}
