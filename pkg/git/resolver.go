package git

import (
	"net/url"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// ResolveRemoteURL detects the git remote URL for the repository at the given
// path. It prefers the current branch's tracking remote, falling back to
// "origin". Returns an empty string if no remote can be determined (not a git
// repo, no remotes configured, etc.). The URL is returned as-is (SSH or HTTPS).
func ResolveRemoteURL(repoPath string) (string, error) {
	repo, err := gogit.PlainOpenWithOptions(repoPath, &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", nil
	}

	remoteName := trackingRemoteName(repo)
	if remoteName == "" {
		remoteName = "origin"
	}

	remote, err := repo.Remote(remoteName)
	if err != nil {
		return "", nil
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", nil
	}
	return stripUserinfo(urls[0]), nil
}

func stripUserinfo(rawURL string) string {
	if strings.Contains(rawURL, "@") && !strings.Contains(rawURL, "://") {
		// SSH-style URL (git@host:path) — no userinfo to strip
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

func trackingRemoteName(repo *gogit.Repository) string {
	head, err := repo.Head()
	if err != nil {
		return ""
	}

	if !head.Name().IsBranch() {
		return ""
	}

	branchName := head.Name().Short()

	cfg, err := repo.Config()
	if err != nil {
		return ""
	}

	branch, ok := cfg.Branches[branchName]
	if !ok {
		return ""
	}

	return branch.Remote
}

// ResolveBranch returns the current branch name for the repository at the given
// path. Returns "main" if the branch cannot be determined (not a git repo,
// detached HEAD, etc.).
func ResolveBranch(repoPath string) string {
	repo, err := gogit.PlainOpenWithOptions(repoPath, &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "main"
	}

	head, err := repo.Head()
	if err != nil {
		return "main"
	}

	if !head.Name().IsBranch() {
		return "main"
	}

	return head.Name().Short()
}
