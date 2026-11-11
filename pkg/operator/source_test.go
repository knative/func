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

// TestSource ensures the Function CR points at the code the deploy built. It
// uses build.source as it is, and fills in what func.yaml leaves empty from
// the git checkout of a local function. The checkout's branch is used only
// when the deploy built the checkout, never for a remote build of a url in
// func.yaml, nor from the directory func runs in.
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
			// A local build deploys the checkout, so the CR follows its branch.
			name:     "local build of a source with no revision",
			f:        fn.Function{Root: checkout, Build: fn.BuildSpec{Source: fn.Source{URL: given.URL}}},
			url:      given.URL,
			revision: "master",
			dir:      ".",
		},
		{
			// The cluster fetches the default branch of the url, so the CR
			// has no revision either.
			name: "remote build of a source with no revision",
			f: fn.Function{Root: checkout, Local: fn.Local{Remote: true},
				Build: fn.BuildSpec{Source: fn.Source{URL: given.URL}}},
			url: given.URL,
			dir: ".",
		},
		{
			// A remote build with no url uploads the checkout.
			name:     "remote build of the checkout",
			f:        fn.Function{Root: checkout, Local: fn.Local{Remote: true}},
			url:      "https://github.com/alice/local.git",
			revision: "master",
			dir:      ".",
		},
		{
			name: "by reference",
			f:    fn.Function{Build: fn.BuildSpec{Source: fn.Source{URL: given.URL}}},
			url:  given.URL,
			dir:  ".",
		},
		{
			name: "source with credentials",
			f:    fn.Function{Build: fn.BuildSpec{Source: fn.Source{URL: "https://user:token@github.com/alice/repo.git"}}},
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
