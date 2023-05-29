package gitlab

import (
	"context"
	"fmt"

	"github.com/xanzy/go-gitlab"
)

type Client struct {
	BaseURL             string
	PersonalAccessToken string
}

func (c Client) CreateWebHook(ctx context.Context, repoOwner, repoName, payloadURL, webhookSecret string) error {
	t := true
	f := false
	glabCli, err := gitlab.NewClient(c.PersonalAccessToken,
		gitlab.WithBaseURL(c.BaseURL),
		gitlab.WithRequestOptions(gitlab.WithContext(ctx)))
	if err != nil {
		panic(err)
	}
	webhook := &gitlab.AddProjectHookOptions{
		EnableSSLVerification: &f,
		PushEvents:            &t,
		Token:                 &webhookSecret,
		URL:                   &payloadURL,
	}
	_, _, err = glabCli.Projects.AddProjectHook(repoOwner+"/"+repoName, webhook)
	if err != nil {
		return fmt.Errorf("cannot create gitlab webhook: %w", err)
	}
	return nil
}
