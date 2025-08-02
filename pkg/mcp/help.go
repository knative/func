package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type template struct {
	Repository   string `json:"repository"`
	Language     string `json:"language"`
	TemplateName string `json:"template"`
}

func fetchTemplates() ([]template, error) {
	var out []template
	seen := make(map[string]bool)

	for _, repoURL := range TEMPLATE_RESOURCE_URIS {
		owner, repo := parseGitHubURL(repoURL)
		api := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/main?recursive=1", owner, repo)

		resp, err := http.Get(api)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var tree struct {
			Tree []struct {
				Path string `json:"path"`
			} `json:"tree"`
		}
		if err := json.Unmarshal(body, &tree); err != nil {
			return nil, err
		}

		for _, item := range tree.Tree {
			parts := strings.Split(item.Path, "/")
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], ".") {
				lang, name := parts[0], parts[1]
				key := lang + "/" + name
				if !seen[key] {
					out = append(out, template{
						Language:     lang,
						TemplateName: name,
						Repository:   repoURL,
					})
					seen[key] = true
				}
			}
		}
	}
	return out, nil
}

func parseGitHubURL(url string) (owner, repo string) {
	trim := strings.TrimPrefix(url, "https://github.com/")
	parts := strings.Split(trim, "/")
	return parts[0], parts[1]
}

func handleRootHelpResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	content, err := exec.Command("func", "--help").Output()
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "func://docs",
			MIMEType: "text/plain",
			Text:     string(content),
		},
	}, nil
}

func runHelpCommand(args []string, uri string) ([]mcp.ResourceContents, error) {
	args = append(args, "--help")
	content, err := exec.Command("func", args...).Output()
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/plain",
			Text:     string(content),
		},
	}, nil
}

func handleListTemplatesResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	templates, err := fetchTemplates()
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "func://templates",
			MIMEType: "text/plain",
			Text:     string(content),
		},
	}, nil
}

func handleCmdHelpPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	cmd := request.Params.Arguments["cmd"]
	if cmd == "" {
		return nil, fmt.Errorf("cmd is required")
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid cmd: %s", cmd)
	}

	return mcp.NewGetPromptResult(
		"Cmd Help Prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent("What can I do with this func command? Please provide help for the command: "+cmd),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewEmbeddedResource(mcp.TextResourceContents{
					URI:      fmt.Sprintf("func://%s/docs", strings.Join(parts, "/")),
					MIMEType: "text/plain",
				}),
			),
		},
	), nil
}

func handleRootHelpPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return mcp.NewGetPromptResult(
		"Help Prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent("What can I do with the func command?"),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewEmbeddedResource(mcp.TextResourceContents{
					URI:      "func://docs",
					MIMEType: "text/plain",
				}),
			),
		},
	), nil
}

func handleListTemplatesPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return mcp.NewGetPromptResult(
		"List Templates Prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent("List available function templates"),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewEmbeddedResource(mcp.TextResourceContents{
					URI:      "func://templates",
					MIMEType: "text/plain",
				}),
			),
		},
	), nil
}
