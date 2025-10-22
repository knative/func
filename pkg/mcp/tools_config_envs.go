package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var configEnvsTool = &mcp.Tool{
	Name:        "config_envs",
	Title:       "Config Environment Variables",
	Description: "Manages environment variable configurations for a function. Can add, remove, or list.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Environment Variables",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  false, // Adding the same environment variable twice or removing a non-existent one will fail.
	},
}

func (s *Server) configEnvsHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsInput) (result *mcp.CallToolResult, output ConfigEnvsOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsOutput{
		Message: string(out),
	}
	return
}

// ConfigEnvsInput defines the input parameters for the config_envs tool.
type ConfigEnvsInput struct {
	Action  string  `json:"action" jsonschema:"required,Action to perform: add, remove, or list"`
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the environment variable"`
	Value   *string `json:"value,omitempty" jsonschema:"Value of the environment variable (for add action)"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsInput) Args() []string {
	args := []string{"envs"}

	// allow "list" as alias for the default action
	if i.Action != "list" {
		args = append(args, i.Action)
	}
	args = append(args, "--path", i.Path) // required
	args = appendStringFlag(args, "--name", i.Name)
	args = appendStringFlag(args, "--value", i.Value)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// ConfigEnvsOutput defines the structured output returned by the config_envs tool.
type ConfigEnvsOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
