package github

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

type Client struct {
	PersonalAccessToken string
}

func (c Client) CreateWebHook(ctx context.Context, repoOwner, repoName, payloadURL, webhookSecret string) error {
	hook := &github.Hook{
		Name:   github.Ptr("web"),
		Active: github.Ptr(true),
		Events: []string{
			"issue_comment",
			"pull_request",
			"push",
		},
		Config: &github.HookConfig{
			URL:         github.Ptr(payloadURL),
			ContentType: github.Ptr("json"),
			InsecureSSL: github.Ptr("1"), // TODO fix insecure (default should be 0)
			Secret:      github.Ptr(webhookSecret),
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
	gprovider, err := github.NewClient(oauth2.NewClient(ctx, ts)).WithEnterpriseURLs(ghApiURL, "")
	if err != nil {
		return nil, err
	}
	return gprovider, nil
}
