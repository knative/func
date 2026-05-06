package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_envs_list

var configEnvsListTool = &mcp.Tool{
	Name:        "config_envs_list",
	Title:       "Config Environment Variables List",
	Description: "Lists the environment variables configured for a function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Config Environment Variables List",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

type ConfigEnvsListInput struct {
	Path    string `json:"path" jsonschema:"required,Path to the function project directory"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsListInput) Args() []string {
	args := []string{"envs", "--path", i.Path}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsListHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsListInput) (result *mcp.CallToolResult, output ConfigEnvsListOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsListOutput{Message: string(out)}
	return
}

// config_envs_add

var configEnvsAddTool = &mcp.Tool{
	Name:        "config_envs_add",
	Title:       "Config Environment Variables Add",
	Description: "Adds an environment variable to a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Environment Variables Add",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // additive only; does not overwrite or delete
		IdempotentHint:  false,      // adding the same variable twice will fail
	},
}

type ConfigEnvsAddInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the environment variable"`
	Value   *string `json:"value,omitempty" jsonschema:"Value of the environment variable"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsAddInput) Args() []string {
	args := []string{"envs", "add", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)
	args = appendStringFlag(args, "--value", i.Value)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsAddOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsAddHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsAddInput) (result *mcp.CallToolResult, output ConfigEnvsAddOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsAddOutput{Message: string(out)}
	return
}

// config_envs_remove

var configEnvsRemoveTool = &mcp.Tool{
	Name:        "config_envs_remove",
	Title:       "Config Environment Variables Remove",
	Description: "Removes an environment variable from a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Environment Variables Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // removes data irreversibly from function config
		IdempotentHint:  false,     // removing a non-existent variable will fail
	},
}

type ConfigEnvsRemoveInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the environment variable to remove"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsRemoveInput) Args() []string {
	args := []string{"envs", "remove", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsRemoveInput) (result *mcp.CallToolResult, output ConfigEnvsRemoveOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsRemoveOutput{Message: string(out)}
	return
}
