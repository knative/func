package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var listTool = &mcp.Tool{
	Name:        "list",
	Title:       "List Functions",
	Description: "Lists all deployed functions in the current namespace, specified namespace, or all namespaces.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "List Functions",
		ReadOnlyHint:   true,
		IdempotentHint: true, // Listing functions with the same parameters multiple times returns consistent results at any point in time.
	},
}

func (s *Server) listHandler(ctx context.Context, r *mcp.CallToolRequest, input ListInput) (result *mcp.CallToolResult, output ListOutput, err error) {
	out, err := s.executor.Execute(ctx, "list", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ListOutput{
		Message: string(out),
	}
	return
}

// ListInput defines the input parameters for the list tool.
// All fields are optional since list can work without any parameters.
type ListInput struct {
	AllNamespaces *bool   `json:"allNamespaces,omitempty" jsonschema:"List functions in all namespaces (overrides namespace parameter)"`
	Namespace     *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to list functions in (default: current namespace)"`
	Output        *string `json:"output,omitempty" jsonschema:"Output format: human, plain, json, xml, or yaml"`
	Verbose       *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ListInput) Args() []string {
	args := []string{}

	args = appendBoolFlag(args, "--all-namespaces", i.AllNamespaces)
	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendStringFlag(args, "--output", i.Output)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// ListOutput defines the structured output returned by the list tool.
type ListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
