package functions

import (
	"context"

	"github.com/go-git/go-git/v5"
)

// GitCommit returns the short commit SHA of the git repository containing
// the given directory. Returns "<sha>-dirty" if the working tree has
// uncommitted changes. Returns an empty string if the directory is not
// inside a git repository.
func GitCommit(dir string) (string, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", nil
	}

	head, err := repo.Head()
	if err != nil {
		return "", nil
	}

	sha := head.Hash().String()[:7]

	wt, err := repo.Worktree()
	if err != nil {
		return sha, nil
	}

	status, err := wt.Status()
	if err != nil {
		return sha, nil
	}

	if !status.IsClean() {
		sha += "-dirty"
	}

	return sha, nil
}

// GitRemoteCommit returns the short commit SHA that g.Revision of the
// repository at g.URL currently resolves to, without a local checkout. It is
// the remote counterpart of GitCommit, so a build from a git repository can
// be labelled with the commit that is actually built.
func GitRemoteCommit(ctx context.Context, g Git) (string, error) {
	src, err := resolveGitSource(ctx, g)
	if err != nil {
		return "", err
	}
	if src.hash.IsZero() {
		return "", nil
	}
	return src.hash.String()[:7], nil
}
