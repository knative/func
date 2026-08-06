package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var deleteTool = &mcp.Tool{
	Name:        "delete",
	Title:       "Delete Function",
	Description: "Delete a deployed Function from the cluster (but not locally).",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Delete Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  true,
	},
}

func (s *Server) deleteHandler(ctx context.Context, r *mcp.CallToolRequest, input DeleteInput) (result *mcp.CallToolResult, output DeleteOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode.  Please set FUNC_ENABLE_MCP_WRITE and restart the client")
		return
	}

	if (input.Path != nil && input.Name != nil) || (input.Path == nil && input.Name == nil) {
		err = fmt.Errorf("exactly one of 'path' or 'name' must be provided")
		return
	}

	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Delete(ctx, input)
	return
}

// DeleteInput defines the input parameters for the delete tool.
type DeleteInput struct {
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the function to delete (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to delete from (default: current or active namespace)"`
	All       *bool   `json:"all,omitempty" jsonschema:"Delete all related resources like Pipelines, Secrets"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

// DeleteOutput defines the structured output returned by the delete tool.
type DeleteOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
