package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var describeTool = &mcp.Tool{
	Name:        "describe",
	Title:       "Describe Function",
	Description: "Print the name, image, namespace, routes, and event subscriptions for a deployed function. Accepts either a local path (--path) or a function name.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Describe Function",
		ReadOnlyHint:   true,
		IdempotentHint: true, // Describing the same function multiple times returns the same result at any point in time.
	},
}

func (s *Server) describeHandler(ctx context.Context, r *mcp.CallToolRequest, input DescribeInput) (result *mcp.CallToolResult, output DescribeOutput, err error) {
	if input.Path != nil && input.Name != nil {
		err = fmt.Errorf("'path' and 'name' are mutually exclusive: provide one or the other")
		return
	}
	if input.Namespace != nil && input.Name == nil {
		err = fmt.Errorf("'namespace' requires 'name' to also be provided")
		return
	}

	out, err := s.executor.Execute(ctx, "describe", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = DescribeOutput{
		Message: string(out),
	}
	return
}

// DescribeInput defines the input parameters for the describe tool.
// Exactly one of Path or Name should be provided; if neither is given, the
// function in the current working directory is described.
type DescribeInput struct {
	// Path and Name are mutually exclusive. Namespace is only valid with Name.
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the function to describe (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace (only used together with name)"`
	Output    *string `json:"output,omitempty" jsonschema:"Output format: human, plain, json, xml, yaml, or url"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i DescribeInput) Args() []string {
	args := []string{}

	// Name is a positional argument; path is a flag.
	if i.Name != nil {
		args = append(args, *i.Name)
	} else if i.Path != nil {
		args = append(args, "--path", *i.Path)
	}

	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendStringFlag(args, "--output", i.Output)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// DescribeOutput defines the structured output returned by the describe tool.
type DescribeOutput struct {
	Message string `json:"message" jsonschema:"Output message from func describe"`
}
