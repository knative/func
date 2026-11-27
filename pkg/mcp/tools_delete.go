package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/utils"
)

var deleteTool = &mcp.Tool{
	Name:        "delete",
	Title:       "Delete Function",
	Description: "Delete a deployed Function from the cluster (but not locally).",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Delete Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  true, // Deleting the same function multiple times results in the same end state (function doesn't exist).
	},
}

func (s *Server) deleteHandler(ctx context.Context, r *mcp.CallToolRequest, input DeleteInput) (result *mcp.CallToolResult, output DeleteOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode. Please set %s and restart the MCP server", EnvMCPWrite)
		return
	}

	// Validate: exactly one of Path or Name must be provided
	if (input.Path != nil && input.Name != nil) || (input.Path == nil && input.Name == nil) {
		err = fmt.Errorf("exactly one of 'path' or 'name' must be provided")
		return
	}
	if input.Path != nil {
		if err = validatePath(*input.Path); err != nil {
			return
		}
	}
	if input.Name != nil {
		if err = utils.ValidateFunctionName(*input.Name); err != nil {
			return
		}
	}

	// Execute
	out, err := s.executor.Execute(ctx, "delete", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = DeleteOutput{
		Message: string(out),
	}
	return
}

// DeleteInput defines the input parameters for the delete tool.
// Exactly one of Path or Name must be provided.
type DeleteInput struct {
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the function to delete (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to delete from (default: current or active namespace)"`
	All       *bool   `json:"all,omitempty" jsonschema:"Delete all related resources like Pipelines, Secrets"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i DeleteInput) Args() []string {
	args := []string{}

	// Either the path flag, or the name as the last positional argument (below)
	if i.Path != nil {
		args = append(args, "--path", *i.Path)
	}

	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendBoolFlag(args, "--all", i.All)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	// The name goes last, after "--", so it can never be parsed as a flag.
	if i.Path == nil && i.Name != nil {
		args = appendPositional(args, *i.Name)
	}
	return args
}

// DeleteOutput defines the structured output returned by the delete tool.
type DeleteOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
