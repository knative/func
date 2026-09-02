package tekton

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// newGitRepo creates a repository with a single commit of a func.yaml and
// returns its path and the full hash of that commit.
func newGitRepo(t *testing.T) (dir, commit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("No 'git' found in path. Skipping test.")
	}
	dir = t.TempDir()
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
	content := "specVersion: " + fn.LastSpecVersion() + "\nname: f\nruntime: go\ncreated: 2024-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "func.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init", "-q", "-b", "main")
	run("add", ".")
	run("commit", "-q", "-m", "initial")
	return dir, run("rev-parse", "HEAD")
}

// Test_sourceCommit ensures the image is labelled with the commit of the
// source the pipeline builds: the git revision when a repository is set,
// even from within an unrelated local checkout, and the local checkout
// otherwise.
func Test_sourceCommit(t *testing.T) {
	remoteDir, remoteCommit := newGitRepo(t)
	localDir, localCommit := newGitRepo(t)

	tests := []struct {
		name string
		f    fn.Function
		want string
	}{
		{"git source without a local checkout",
			fn.Function{Build: fn.BuildSpec{Git: fn.Git{URL: "file://" + remoteDir}}},
			remoteCommit[:7]},
		{"git source wins over a local checkout",
			fn.Function{Root: localDir, Build: fn.BuildSpec{Git: fn.Git{URL: "file://" + remoteDir}}},
			remoteCommit[:7]},
		{"local checkout",
			fn.Function{Root: localDir},
			localCommit[:7]},
		{"no source information",
			fn.Function{Root: t.TempDir()},
			""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sourceCommit(context.Background(), tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("expected commit %q, got %q", tt.want, got)
			}
		})
	}

	t.Run("unknown revision is an error", func(t *testing.T) {
		f := fn.Function{Build: fn.BuildSpec{Git: fn.Git{URL: "file://" + remoteDir, Revision: "nope"}}}
		if _, err := sourceCommit(context.Background(), f); err == nil {
			t.Fatal("expected an error")
		}
	})
}
