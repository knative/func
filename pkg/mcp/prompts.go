package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var funcWorkflowPrompt = &mcp.Prompt{
	Name:        "func-workflow",
	Title:       "Function Lifecycle Workflow",
	Description: "Guides through the full Function lifecycle: create, build, and deploy.",
	Arguments: []*mcp.PromptArgument{
		{
			Name:        "language",
			Description: "Programming language for the Function (e.g. go, python, node).",
		},
	},
}

func funcWorkflowHandler(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	lang := req.Params.Arguments["language"]
	return &mcp.GetPromptResult{
		Description: "Step-by-step guide for the Function lifecycle.",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: funcWorkflowText(lang)},
			},
		},
	}, nil
}

func funcWorkflowText(language string) string {
	langLine := ""
	if language != "" {
		langLine = fmt.Sprintf("\nUse language: %s\n", language)
	}
	return fmt.Sprintf(`Guide me through the full Function lifecycle.%s
Step 1 - Create:
  Use the "create" tool to scaffold a new Function.
  Check "func://languages" for available languages and "func://templates" for templates.

Step 2 - Build:
  Use the "build" tool to compile the Function into a container image.

Step 3 - Deploy:
  Use the "deploy" tool to deploy the Function to the cluster.
  On first deploy, a container registry is required (e.g. docker.io/user).

Use "func://function" to check current Function state at any point.`, langLine)
}
