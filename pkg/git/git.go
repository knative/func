package git

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/formatting"
)

const (
	GitHubProvider    = "github"
	GitLabProvider    = "gitlab"
	BitBucketProvider = "bitbucket-cloud"
)

type SupportedProviders []string

var SupportedProvidersList = SupportedProviders{GitHubProvider}

func (sp SupportedProviders) PrettyString() string {
	var b strings.Builder
	for i, v := range sp {
		if i < len(sp)-2 {
			b.WriteString(strconv.Quote(v) + ", ")
		} else if i < len(sp)-1 {
			b.WriteString(strconv.Quote(v) + " and ")
		} else {
			b.WriteString(strconv.Quote(v))
		}
	}
	return b.String()
}

func GitProviderName(url string) (string, error) {
	switch {
	case strings.Contains(url, "github"):
		return GitHubProvider, nil
	case strings.Contains(url, "gitlab"):
		//return GitLabProvider, nil
	case strings.Contains(url, "bitbucket-cloud"):
		//return BitBucketProvider, nil
	}
	return "", fmt.Errorf("runtime for url %q is not supported, please use one of supported runtimes: %s", url, SupportedProvidersList.PrettyString())
}

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
