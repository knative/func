package git

import (
	"fmt"
	"strings"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/formatting"
)

// RepoOwnerAndNameFromUrl for input url returns repo owner and repo name
// eg. for github.com/foo/bar returns 'foo' and 'bar'
func RepoOwnerAndNameFromUrl(url string) (string, string, error) {

	defaultRepo, err := formatting.GetRepoOwnerFromURL(url)
	if err != nil {
		return "", "", err
	}

	defaultRepo = strings.TrimSuffix(defaultRepo, "/")
	repoArr := strings.Split(defaultRepo, "/")
	if len(repoArr) != 2 {
		return "", "", fmt.Errorf("invalid repository, needs to be of format 'org-name/repo-name'")
	}
	repoOwner := repoArr[0]
	repoName := repoArr[1]

	return repoOwner, repoName, nil
}
