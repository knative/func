package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var createTool = &mcp.Tool{
	Name:        "create",
	Title:       "Create Function",
	Description: "Create a new Function project.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Create Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  false,
	},
}

func (s *Server) createHandler(ctx context.Context, r *mcp.CallToolRequest, input CreateInput) (result *mcp.CallToolResult, output CreateOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Create(ctx, input)
	return
}

type CreateInput struct {
	Language   string  `json:"language" jsonschema:"required,Language runtime to use"`
	Path       string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Template   *string `json:"template,omitempty" jsonschema:"Function template (e.g., http, cloudevents)"`
	Repository *string `json:"repository,omitempty" jsonschema:"Git repository URI containing custom templates"`
	Verbose    *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type CreateOutput struct {
	Runtime  string  `json:"runtime" jsonschema:"Language runtime used"`
	Template *string `json:"template" jsonschema:"Template used"`
	Message  string  `json:"message,omitempty" jsonschema:"Output message"`
}
