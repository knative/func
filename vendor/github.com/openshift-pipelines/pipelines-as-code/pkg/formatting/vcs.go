package formatting

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// SanitizeBranch remove refs/heads from string, only removing the first prefix
// in case we have branch that are actually called refs-heads 🙃
func SanitizeBranch(s string) string {
	if strings.HasPrefix(s, "refs/heads/") {
		return strings.TrimPrefix(s, "refs/heads/")
	}
	if strings.HasPrefix(s, "refs-heads-") {
		return strings.TrimPrefix(s, "refs-heads-")
	}
	return s
}

// ShortSHA returns a shortsha
func ShortSHA(sha string) string {
	if sha == "" {
		return ""
	}
	if shortShaLength >= len(sha)+1 {
		return sha
	}
	return sha[0:shortShaLength]
}

func GetRepoOwnerFromURL(ghURL string) (string, error) {
	org, repo, err := GetRepoOwnerSplitted(ghURL)
	if err != nil {
		return "", err
	}
	repo = strings.TrimSuffix(repo, "/")
	return strings.ToLower(fmt.Sprintf("%s/%s", org, repo)), nil
}

func GetRepoOwnerSplitted(u string) (string, string, error) {
	uparse, err := url.Parse(u)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(uparse.Path, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid repo url at least a organization/project and a repo needs to be specified: %s", u)
	}
	org := filepath.Join(parts[0 : len(parts)-1]...)
	repo := parts[len(parts)-1]
	return org, repo, nil
}

// CamelCasit pull_request > PullRequest
func CamelCasit(s string) string {
	c := cases.Title(language.AmericanEnglish)
	return strings.ReplaceAll(c.String(strings.ReplaceAll(s, "_", " ")), " ", "")
}
