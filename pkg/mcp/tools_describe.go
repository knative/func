package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var describeTool = &mcp.Tool{
	Name:        "describe",
	Title:       "Describe Function",
	Description: "Show the name, route, subscriptions (extractors), and other details of a deployed function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Describe Function",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) describeHandler(ctx context.Context, r *mcp.CallToolRequest, input DescribeInput) (result *mcp.CallToolResult, output DescribeOutput, err error) {
	// Validate: path and name are mutually exclusive
	if input.Path != nil && input.Name != nil {
		err = fmt.Errorf("'path' and 'name' are mutually exclusive")
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
// Path and Name are mutually exclusive. If neither is provided, the function
// in the current directory is described.
type DescribeInput struct {
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the deployed function to describe (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the function (default: current namespace)"`
	Output    *string `json:"output,omitempty" jsonschema:"Output format: human, plain, json, xml, or yaml"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i DescribeInput) Args() []string {
	args := []string{}

	// Either path flag or positional name argument
	if i.Path != nil {
		args = append(args, "--path", *i.Path)
	} else if i.Name != nil {
		args = append(args, *i.Name)
	}

	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendStringFlag(args, "--output", i.Output)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// DescribeOutput defines the structured output returned by the describe tool.
type DescribeOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
