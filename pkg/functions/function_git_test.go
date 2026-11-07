package functions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// gitFixture is a local repository served over file:// with:
//   - main: func.yaml naming "root-fn" and sub/func.yaml naming "sub-fn"
//   - tag v1 (annotated) on the first commit of main
//   - branch feature: func.yaml naming "feature-fn"
//   - branch v1, at feature's head: the same name as the tag, another commit
//   - branch legacy: a func.yaml from spec version 0.25.0, needing migration
type gitFixture struct {
	url     string
	main    string // full hash of main's head
	feature string // full hash of feature's head
}

func newGitFixture(t *testing.T) gitFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		// The fixture is served over file://, which go-git only supports
		// through the git binary and not from a Windows path.
		t.Skip("file:// repositories are not supported on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("No 'git' found in path. Skipping test.")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(rel, name string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "specVersion: " + fn.LastSpecVersion() + "\nname: " + name + "\nruntime: go\ncreated: 2024-01-01T00:00:00Z\n"
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	// Let the test fetch a bare commit hash, as the common git hosts allow.
	run("config", "uploadpack.allowAnySHA1InWant", "true")
	write("func.yaml", "root-fn")
	write("sub/func.yaml", "sub-fn")
	run("add", ".")
	run("commit", "-q", "-m", "initial")
	run("tag", "-a", "v1", "-m", "v1")
	main := run("rev-parse", "HEAD")

	run("checkout", "-q", "-b", "feature")
	write("func.yaml", "feature-fn")
	run("commit", "-q", "-am", "feature")
	feature := run("rev-parse", "HEAD")
	run("branch", "v1") // same name as the tag, a different commit

	run("checkout", "-q", "-b", "legacy", "main")
	legacy, err := os.ReadFile(filepath.Join("testdata", "migrations", "v0.34.0", "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "func.yaml"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-q", "-am", "legacy")
	run("checkout", "-q", "main")

	return gitFixture{url: "file://" + dir, main: main, feature: feature}
}

// TestNewFunctionFromGit ensures a function is loaded from the func.yaml of
// the requested revision and context directory without a local checkout.
func TestNewFunctionFromGit(t *testing.T) {
	fx := newGitFixture(t)
	tests := []struct {
		name       string
		git        fn.Source
		wantName   string
		wantCommit string
	}{
		{"default branch", fn.Source{URL: fx.url}, "root-fn", fx.main},
		{"context dir", fn.Source{URL: fx.url, Dir: "sub"}, "sub-fn", fx.main},
		{"branch", fn.Source{URL: fx.url, Revision: "feature"}, "feature-fn", fx.feature},
		{"annotated tag", fn.Source{URL: fx.url, Revision: "v1"}, "root-fn", fx.main},
		{"full ref", fn.Source{URL: fx.url, Revision: "refs/heads/feature"}, "feature-fn", fx.feature},
		{"commit hash", fn.Source{URL: fx.url, Revision: fx.feature}, "feature-fn", fx.feature},
		// A tag wins over a branch of the same name, as for git; the full ref
		// name chooses the branch
		{"tag over branch", fn.Source{URL: fx.url, Revision: "v1"}, "root-fn", fx.main},
		{"branch by full ref", fn.Source{URL: fx.url, Revision: "refs/heads/v1"}, "feature-fn", fx.feature},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := fn.NewFunctionFromGit(context.Background(), tt.git)
			if err != nil {
				t.Fatal(err)
			}
			if f.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, f.Name)
			}
			if f.Build.Source.Commit != tt.wantCommit {
				t.Errorf("expected the function read at %s, got %q", tt.wantCommit, f.Build.Source.Commit)
			}
			// The function records where it was read from
			want := tt.git
			want.Commit = tt.wantCommit
			if f.Build.Source != want {
				t.Errorf("expected source %+v, got %+v", want, f.Build.Source)
			}
			if f.Runtime != "go" {
				t.Errorf("expected runtime go, got %q", f.Runtime)
			}
			if f.Root != "" {
				t.Errorf("expected no root, got %q", f.Root)
			}
			if !f.Initialized() {
				t.Error("expected the function to be initialized")
			}
		})
	}
}

// TestNewFunctionFromGit_Migrates ensures a func.yaml of an earlier spec
// version is migrated on load, as NewFunction does for a local checkout:
// migrations read the previous structure from the fetched bytes.
func TestNewFunctionFromGit_Migrates(t *testing.T) {
	fx := newGitFixture(t)
	f, err := fn.NewFunctionFromGit(context.Background(), fn.Source{URL: fx.url, Revision: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if f.SpecVersion != fn.LastSpecVersion() {
		t.Errorf("expected spec version %q, got %q", fn.LastSpecVersion(), f.SpecVersion)
	}
	if f.Name != "testfunc" {
		t.Errorf("expected name testfunc, got %q", f.Name)
	}
	// The legacy func.yaml names another repository (http://test-url, moved
	// into the source by migrateToSpecsStructure). The loaded function
	// records where it was actually read from instead.
	if f.Build.Source.URL != fx.url || f.Build.Source.Revision != "legacy" {
		t.Errorf("expected the source it was read from (%s at legacy), got %+v", fx.url, f.Build.Source)
	}
}

// TestNewFunctionFromGit_Errors ensures unknown revisions and directories
// without a func.yaml are reported, not silently returned as empty functions.
func TestNewFunctionFromGit_Errors(t *testing.T) {
	fx := newGitFixture(t)
	tests := []struct {
		name    string
		git     fn.Source
		wantErr string
	}{
		{"unknown revision", fn.Source{URL: fx.url, Revision: "nope"}, "not found"},
		{"unknown full ref", fn.Source{URL: fx.url, Revision: "refs/heads/nope"}, "couldn't find remote ref"},
		{"abbreviated hash", fn.Source{URL: fx.url, Revision: fx.feature[:7]}, "full hash"},
		{"missing func.yaml", fn.Source{URL: fx.url, Dir: "nope"}, "no func.yaml"},
		{"no url", fn.Source{}, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fn.NewFunctionFromGit(context.Background(), tt.git)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err)
			}
		})
	}
}
