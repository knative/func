package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Info struct {
	URL          string
	TopLevelPath string
	SHA          string
	Branch       string
}

func RunGit(dir string, args ...string) (string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		//nolint: nilerr
		return "", nil
	}
	c := exec.Command(gitPath, args...)
	var output bytes.Buffer
	c.Stderr = &output
	c.Stdout = &output
	// This is the optional working directory. If not set, it defaults to the current
	// working directory of the process.
	if dir != "" {
		c.Dir = dir
	}
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("error running, %s, output: %s error: %w", args, output.String(), err)
	}
	return output.String(), nil
}

// GetGitInfo try to detect the current remote for this URL return the origin url transformed and the topdir
func GetGitInfo(dir string) *Info {
	gitURL, err := RunGit(dir, "remote", "get-url", "origin")
	if err != nil {
		gitURL, err = RunGit(dir, "remote", "get-url", "upstream")
		if err != nil {
			return &Info{}
		}
	}
	gitURL = strings.TrimSpace(gitURL)
	gitURL = strings.TrimSuffix(gitURL, ".git")

	// convert github and probably others ssh access format into https
	// i think it only fails with bitbucket server
	if strings.HasPrefix(gitURL, "git@") {
		sp := strings.Split(gitURL, ":")
		prefix := strings.ReplaceAll(sp[0], "git@", "https://")
		gitURL = fmt.Sprintf("%s/%s", prefix, strings.Join(sp[1:], ":"))
	}

	brootdir, err := RunGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return &Info{}
	}

	sha, err := RunGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return &Info{}
	}

	headbranch, err := RunGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return &Info{}
	}

	return &Info{
		URL:          gitURL,
		TopLevelPath: strings.TrimSpace(brootdir),
		SHA:          strings.TrimSpace(sha),
		Branch:       strings.TrimSpace(headbranch),
	}
}
