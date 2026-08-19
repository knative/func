package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var logsTool = &mcp.Tool{
	Name:        "logs",
	Title:       "Function Logs",
	Description: "Retrieve recent logs from a deployed Function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Function Logs",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) logsHandler(ctx context.Context, r *mcp.CallToolRequest, input LogsInput) (result *mcp.CallToolResult, output LogsOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Logs(ctx, input)
	return
}

type LogsInput struct {
	Path      string  `json:"path" jsonschema:"Path to the function project directory (used when name is not provided)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the deployed function"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace"`
	Since     *string `json:"since,omitempty" jsonschema:"Return logs newer than a relative duration (e.g. 5m, 1h)"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type LogsOutput struct {
	Message string `json:"message" jsonschema:"Log output"`
}
