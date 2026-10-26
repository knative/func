package operator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	fn "knative.dev/func/pkg/functions"
)

// TestSource ensures the Function CR points at the function's own source:
// the git checkout of a local function, and for a function deployed by
// reference only the source it was deployed from, whatever git repository the
// current directory is in.
func TestSource(t *testing.T) {
	checkout := t.TempDir()
	repo, err := gogit.PlainInit(checkout, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/alice/local.git"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(checkout, "README.md"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(checkout)

	given := fn.Source{URL: "https://github.com/alice/repo.git", Revision: "v1", Dir: "fn"}
	tests := []struct {
		name               string
		f                  fn.Function
		url, revision, dir string
	}{
		{
			name:     "local function",
			f:        fn.Function{Root: checkout},
			url:      "https://github.com/alice/local.git",
			revision: "master",
			dir:      ".",
		},
		{
			name:     "local function with a source",
			f:        fn.Function{Root: checkout, Build: fn.BuildSpec{Source: given}},
			url:      given.URL,
			revision: given.Revision,
			dir:      given.Dir,
		},
		{
			name: "by reference",
			f:    fn.Function{Build: fn.BuildSpec{Source: fn.Source{URL: given.URL}}},
			url:  given.URL,
			dir:  ".",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, revision, dir, err := source(tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if url != tt.url || revision != tt.revision || dir != tt.dir {
				t.Errorf("expected %q %q %q, got %q %q %q", tt.url, tt.revision, tt.dir, url, revision, dir)
			}
		})
	}
}
