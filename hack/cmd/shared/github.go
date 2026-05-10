package shared

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// NewGHClient constructs an authenticated GitHub client using GITHUB_TOKEN.
func NewGHClient(ctx context.Context) *github.Client {
	return github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: os.Getenv("GITHUB_TOKEN"),
	})))
}

// RepoFromEnv splits GITHUB_REPOSITORY ("owner/repo") into owner and repo.
func RepoFromEnv() (owner, repo string, err error) {
	v := os.Getenv("GITHUB_REPOSITORY")
	if v == "" {
		return "", "", fmt.Errorf("GITHUB_REPOSITORY is not set")
	}
	parts := strings.SplitN(v, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("GITHUB_REPOSITORY has unexpected format: %q", v)
	}
	return parts[0], parts[1], nil
}

// PRExists returns true if any open pull request satisfies pred(pr.GetTitle()).
func PRExists(ctx context.Context, client *github.Client, owner, repo string, pred func(string) bool) (bool, error) {
	opts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 10,
		},
	}
	for {
		prs, resp, err := client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return false, fmt.Errorf("cannot list pull requests: %w", err)
		}
		for _, pr := range prs {
			if pred(pr.GetTitle()) {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return false, nil
}

// PrepareBranch configures git identity, creates branchName, runs codegen,
// stages filesToAdd, commits with prTitle as the message, and pushes.
func PrepareBranch(ctx context.Context, branchName, prTitle string, filesToAdd []string) error {
	steps := [][]string{
		{"git", "config", "user.email", "automation@knative.team"},
		{"git", "config", "user.name", "Knative Automation"},
		{"git", "checkout", "-b", branchName},
		{"make", "generate/zz_filesystem_generated.go"},
	}
	for _, args := range steps {
		if err := RunCmd(ctx, args[0], args[1:]...); err != nil {
			return err
		}
	}

	addArgs := append([]string{"add"}, filesToAdd...)
	if err := RunCmd(ctx, "git", addArgs...); err != nil {
		return err
	}

	if err := RunCmd(ctx, "git", "commit", "-m", prTitle); err != nil {
		return err
	}

	return RunCmd(ctx, "git", "push", "--set-upstream", "origin", branchName)
}

// CreatePR opens a pull request against main with the given title.
// head should be in the form "owner:branchName".
func CreatePR(ctx context.Context, client *github.Client, owner, repo, title, head string) error {
	_, _, err := client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.Ptr(title),
		Body:  github.Ptr(title),
		Base:  github.Ptr("main"),
		Head:  github.Ptr(head),
	})
	if err != nil {
		return fmt.Errorf("cannot create pull request: %w", err)
	}
	return nil
}

// RunCmd runs name with args, streaming stdout/stderr to the process output.
func RunCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", name+" "+strings.Join(args, " "), err)
	}
	return nil
}
