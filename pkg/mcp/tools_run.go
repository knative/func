package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var runTool = &mcp.Tool{
	Name:        "run",
	Title:       "Run Function",
	Description: "Run a Function locally for development and testing.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Run Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  false,
	},
}

func (s *Server) runHandler(ctx context.Context, r *mcp.CallToolRequest, input RunInput) (result *mcp.CallToolResult, output RunOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Run(ctx, input)
	return
}

type RunInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Address *string `json:"address,omitempty" jsonschema:"Host:port to bind (e.g. 127.0.0.1:8080)"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type RunOutput struct {
	URL     string `json:"url,omitempty" jsonschema:"URL where the function is listening"`
	Address string `json:"address,omitempty" jsonschema:"Host:port address"`
	Message string `json:"message" jsonschema:"Status message"`
}
