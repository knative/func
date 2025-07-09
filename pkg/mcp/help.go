package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

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
