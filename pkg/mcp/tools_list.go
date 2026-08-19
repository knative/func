package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var listTool = &mcp.Tool{
	Name:        "list",
	Title:       "List Functions",
	Description: "Lists all deployed functions in the current namespace, specified namespace, or all namespaces.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "List Functions",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) listHandler(ctx context.Context, r *mcp.CallToolRequest, input ListInput) (result *mcp.CallToolResult, output ListOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.List(ctx, input)
	return
}

// ListInput defines the input parameters for the list tool.
type ListInput struct {
	AllNamespaces *bool   `json:"allNamespaces,omitempty" jsonschema:"List functions in all namespaces (overrides namespace parameter)"`
	Namespace     *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to list functions in (default: current namespace)"`
	Output        *string `json:"output,omitempty" jsonschema:"Output format: human, plain, json, xml, or yaml"`
	Verbose       *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

// ListOutput defines the structured output returned by the list tool.
type ListOutput struct {
	Message   string         `json:"message" jsonschema:"Output message"`
	Functions []fn.ListItem  `json:"functions,omitempty" jsonschema:"Structured list of deployed functions"`
}
