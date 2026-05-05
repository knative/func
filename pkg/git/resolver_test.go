package git

import (
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestResolveRemoteURL_OriginRemote(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/alice/my-func.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/alice/my-func.git" {
		t.Fatalf("expected origin URL, got %q", url)
	}
}

func TestResolveRemoteURL_TrackingRemote(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/alice/my-func.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "upstream",
		URLs: []string{"https://github.com/upstream/my-func.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(dir + "/README.md")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err = wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, _ := repo.Config()
	cfg.Branches["master"] = &gogitconfig.Branch{
		Name:   "master",
		Remote: "upstream",
		Merge:  plumbing.ReferenceName("refs/heads/master"),
	}
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/upstream/my-func.git" {
		t.Fatalf("expected tracking remote URL, got %q", url)
	}
}

func TestResolveRemoteURL_NoRemotes(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "" {
		t.Fatalf("expected empty URL, got %q", url)
	}
}

func TestResolveRemoteURL_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "" {
		t.Fatalf("expected empty URL, got %q", url)
	}
}

func TestResolveRemoteURL_SSHPassThrough(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"git@github.com:alice/my-func.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "git@github.com:alice/my-func.git" {
		t.Fatalf("expected SSH URL passed through, got %q", url)
	}
}

func TestResolveRemoteURL_StripsUsername(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"http://admin@172.18.0.2:30000/admin/my-func"},
	})
	if err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://172.18.0.2:30000/admin/my-func" {
		t.Fatalf("expected userinfo stripped, got %q", url)
	}
}

func TestResolveRemoteURL_StripsUsernameAndPassword(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://user:token@github.com/alice/my-func.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	url, err := ResolveRemoteURL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/alice/my-func.git" {
		t.Fatalf("expected credentials stripped, got %q", url)
	}
}

func TestResolveBranch_CurrentBranch(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(dir + "/README.md")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err = wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	branch := ResolveBranch(dir)
	if branch != "master" {
		t.Fatalf("expected 'master', got %q", branch)
	}
}

func TestResolveBranch_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	branch := ResolveBranch(dir)
	if branch != "main" {
		t.Fatalf("expected default 'main', got %q", branch)
	}
}
