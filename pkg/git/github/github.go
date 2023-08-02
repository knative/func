package github

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-github/v49/github"
	"golang.org/x/oauth2"
)

type Client struct {
	PersonalAccessToken string
}

func (c Client) CreateWebHook(ctx context.Context, repoOwner, repoName, payloadURL, webhookSecret string) error {
	hook := &github.Hook{
		Name:   github.String("web"),
		Active: github.Bool(true),
		Events: []string{
			"issue_comment",
			"pull_request",
			"push",
		},
		Config: map[string]interface{}{
			"url":          payloadURL,
			"content_type": "json",
			"insecure_ssl": "1", // TODO fix insecure (default should be 0)
			"secret":       webhookSecret,
		},
	}

	ghClient, err := newGHClientByToken(ctx, c.PersonalAccessToken, "")
	if err != nil {
		return err
	}

	_, res, err := ghClient.Repositories.CreateHook(ctx, repoOwner, repoName, hook)
	if err != nil {
		return err
	}

	if res.Response.StatusCode != http.StatusCreated {
		payload, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		return fmt.Errorf("failed to create webhook on repository %v/%v, status code: %v, error : %v",
			repoOwner, repoName, res.Response.StatusCode, payload)
	}

	return nil
}

func newGHClientByToken(ctx context.Context, personalAccessToken, ghApiURL string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: personalAccessToken},
	)

	if ghApiURL == "" || ghApiURL == "https://api.github.com/" {
		return github.NewClient(oauth2.NewClient(ctx, ts)), nil
	}

	// GitHub Enterprise
	gprovider, err := github.NewEnterpriseClient(ghApiURL, "", oauth2.NewClient(ctx, ts))
	if err != nil {
		return nil, err
	}
	return gprovider, nil
}
