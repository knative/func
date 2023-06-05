package git

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/formatting"

	"knative.dev/func/pkg/git/github"
	"knative.dev/func/pkg/git/gitlab"
)

const (
	GitHubProvider    = "github"
	GitLabProvider    = "gitlab"
	BitBucketProvider = "bitbucket-cloud"
)

type SupportedProviders []string

var SupportedProvidersList = SupportedProviders{GitHubProvider, GitLabProvider}

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
		return GitLabProvider, nil
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

	repoName = strings.TrimSuffix(repoName, ".git")

	return repoOwner, repoName, nil
}

func CreateWebHook(ctx context.Context, gitRepoURL, webHookTarget, webHookSecret, personalAccessToken string) error {
	providerName, err := GitProviderName(gitRepoURL)
	if err != nil {
		return err
	}

	u, err := url.Parse(gitRepoURL)
	if err != nil {
		return fmt.Errorf("cannot parse git repo url: %w", err)
	}

	var cli providerClient
	switch providerName {
	case GitHubProvider:
		cli = github.Client{
			PersonalAccessToken: personalAccessToken,
		}
	case GitLabProvider:
		cli = gitlab.Client{
			BaseURL:             u.Scheme + "://" + u.Host,
			PersonalAccessToken: personalAccessToken,
		}
	}

	repoOwner, repoName, err := RepoOwnerAndNameFromUrl(gitRepoURL)
	if err != nil {
		return err
	}

	err = cli.CreateWebHook(ctx, repoOwner, repoName, webHookTarget, webHookSecret)
	if err != nil {
		return fmt.Errorf("cannot create web hook: %w", err)
	}
	return nil
}

type providerClient interface {
	CreateWebHook(ctx context.Context, repoOwner, repoName, payloadURL, webhookSecret string) error
}
