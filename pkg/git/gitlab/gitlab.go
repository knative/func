package gitlab

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type Client struct {
	BaseURL             string
	PersonalAccessToken string
}

func (c Client) CreateWebHook(ctx context.Context, repoOwner, repoName, payloadURL, webhookSecret string) error {
	glabCli, err := gitlab.NewClient(c.PersonalAccessToken,
		gitlab.WithBaseURL(c.BaseURL),
		gitlab.WithRequestOptions(gitlab.WithContext(ctx)))
	if err != nil {
		return fmt.Errorf("cannot create GitLab client: %w", err)
	}

	projectPath := repoOwner + "/" + repoName

	existingHooks, _, err := glabCli.Projects.ListProjectHooks(projectPath, nil)
	if err != nil {
		return fmt.Errorf("cannot list existing hooks: %w", err)
	}
	for _, hook := range existingHooks {
		if hook.URL == payloadURL {
			fmt.Printf("GitLab webhook already exists for project %s at URL: %s\n", projectPath, payloadURL)
			return nil
		}
	}

	webhook := projectHookOpts(payloadURL, webhookSecret)

	_, _, err = glabCli.Projects.AddProjectHook(projectPath, webhook)
	if err != nil {
		return fmt.Errorf("cannot create gitlab webhook: %w", err)
	}
	return nil
}

// projectHookOpts returns REST options used by CreateWebHook with TLS
// certificate verification enabled for the payload URL (GitLab default).
func projectHookOpts(payloadURL, webhookSecret string) *gitlab.AddProjectHookOptions {
	enableSSLVerify := true
	pushEvents := true
	return &gitlab.AddProjectHookOptions{
		EnableSSLVerification: &enableSSLVerify,
		PushEvents:            &pushEvents,
		Token:                 &webhookSecret,
		URL:                   &payloadURL,
	}
}
