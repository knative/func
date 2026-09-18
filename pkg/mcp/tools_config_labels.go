package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
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
	Path string `json:"path" jsonschema:"required,Path to the function project directory"`
}

// LabelPair is the MCP-facing representation of a configured label. It
// mirrors fn.Label without the invopop/jsonschema struct tags, which the MCP
// SDK's output-schema generator cannot parse.
type LabelPair struct {
	Key   *string `json:"key,omitempty" jsonschema:"Label key"`
	Value *string `json:"value,omitempty" jsonschema:"Label value or template expression"`
}

// ConfigLabelsListOutput exposes the configured labels as typed structured
// data; Message is a human-readable summary for fallback display.
type ConfigLabelsListOutput struct {
	Labels  []LabelPair `json:"labels" jsonschema:"Configured labels"`
	Message string      `json:"message" jsonschema:"Human-readable summary"`
}

// configLabelsListHandler loads the function at the given path and returns its
// configured labels directly from pkg/functions rather than shelling out to
// `func config labels`. Part of the migration tracked in
// https://github.com/knative/func/issues/3771.
func (s *Server) configLabelsListHandler(_ context.Context, _ *mcp.CallToolRequest, input ConfigLabelsListInput) (result *mcp.CallToolResult, output ConfigLabelsListOutput, err error) {
	f, err := fn.NewFunction(input.Path)
	if err != nil {
		return
	}
	labels := make([]LabelPair, len(f.Deploy.Labels))
	for i, l := range f.Deploy.Labels {
		labels[i] = LabelPair{Key: l.Key, Value: l.Value}
	}
	output = ConfigLabelsListOutput{
		Labels:  labels,
		Message: formatLabels(f.Deploy.Labels),
	}
	return
}

func formatLabels(labels []fn.Label) string {
	if len(labels) == 0 {
		return "No labels defined"
	}
	var b strings.Builder
	b.WriteString("Labels:\n")
	for _, l := range labels {
		fmt.Fprintf(&b, " -  %s\n", l.String())
	}
	return b.String()
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
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigLabelsRemoveOutput{Message: string(out)}
	return
}
