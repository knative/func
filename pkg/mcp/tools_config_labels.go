package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var configLabelsTool = &mcp.Tool{
	Name:        "config_labels",
	Title:       "Config Labels",
	Description: "Manages label configurations for a function. Can add, remove, or list.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Labels",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  false, // Adding the same label twice or removing a non-existent label will fail.
	},
}

func (s *Server) configLabelsHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigLabelsInput) (result *mcp.CallToolResult, output ConfigLabelsOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigLabelsOutput{
		Message: string(out),
	}
	return
}

// ConfigLabelsInput defines the input parameters for the config_labels tool.
type ConfigLabelsInput struct {
	Action  string  `json:"action" jsonschema:"required,Action to perform: add, remove, or list"`
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the label"`
	Value   *string `json:"value,omitempty" jsonschema:"Value of the label (for add action)"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigLabelsInput) Args() []string {
	args := []string{"labels"}

	// allow "list" as an alias for the default action
	if i.Action != "list" {
		args = append(args, i.Action)
	}
	args = append(args, "--path", i.Path)
	args = appendStringFlag(args, "--name", i.Name)
	args = appendStringFlag(args, "--value", i.Value)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// ConfigLabelsOutput defines the structured output returned by the config_labels tool.
type ConfigLabelsOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
