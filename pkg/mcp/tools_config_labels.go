package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_labels_list

var configLabelsListTool = &mcp.Tool{
	Name:        "config_labels_list",
	Title:       "Config Labels List",
	Description: "Lists the labels configured for a function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Config Labels List",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

type ConfigLabelsListInput struct {
	Path    string `json:"path" jsonschema:"required,Path to the function project directory"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigLabelsListInput) Args() []string {
	args := []string{"labels", "--path", i.Path}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigLabelsListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configLabelsListHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigLabelsListInput) (result *mcp.CallToolResult, output ConfigLabelsListOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigLabelsListOutput{Message: string(out)}
	return
}

// config_labels_add

var configLabelsAddTool = &mcp.Tool{
	Name:        "config_labels_add",
	Title:       "Config Labels Add",
	Description: "Adds a label to a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Labels Add",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // additive only; does not overwrite or delete
		IdempotentHint:  false,      // adding the same label twice will fail
	},
}

type ConfigLabelsAddInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the label"`
	Value   *string `json:"value,omitempty" jsonschema:"Value of the label"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigLabelsAddInput) Args() []string {
	args := []string{"labels", "add", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)
	args = appendStringFlag(args, "--value", i.Value)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigLabelsAddOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configLabelsAddHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigLabelsAddInput) (result *mcp.CallToolResult, output ConfigLabelsAddOutput, err error) {
	if s.readonly.Load() {
		err = errReadOnlyMode
		return
	}
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigLabelsAddOutput{Message: string(out)}
	return
}

// config_labels_remove

var configLabelsRemoveTool = &mcp.Tool{
	Name:        "config_labels_remove",
	Title:       "Config Labels Remove",
	Description: "Removes a label from a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Labels Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // removes data irreversibly from function config
		IdempotentHint:  false,     // removing a non-existent label will fail
	},
}

type ConfigLabelsRemoveInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the label to remove"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigLabelsRemoveInput) Args() []string {
	args := []string{"labels", "remove", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigLabelsRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configLabelsRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigLabelsRemoveInput) (result *mcp.CallToolResult, output ConfigLabelsRemoveOutput, err error) {
	if s.readonly.Load() {
		err = errReadOnlyMode
		return
	}
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigLabelsRemoveOutput{Message: string(out)}
	return
}
