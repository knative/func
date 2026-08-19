package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var describeTool = &mcp.Tool{
	Name:        "describe",
	Title:       "Describe Function",
	Description: "Describe a deployed Function's route, image, and status.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Describe Function",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) describeHandler(ctx context.Context, r *mcp.CallToolRequest, input DescribeInput) (result *mcp.CallToolResult, output DescribeOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Describe(ctx, input)
	return
}

type DescribeInput struct {
	Path      string  `json:"path" jsonschema:"Path to the function project directory (used when name is not provided)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the deployed function"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace"`
	Output    *string `json:"output,omitempty" jsonschema:"Output format: human or json"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type DescribeOutput struct {
	Instance fn.Instance `json:"instance,omitempty" jsonschema:"Structured function instance details"`
	Message  string      `json:"message" jsonschema:"Human-readable description"`
}
