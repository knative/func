package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var createTool = &mcp.Tool{
	Name:        "create",
	Title:       "Create Function",
	Description: "Create a new Function project.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Create Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  false, // Running create twice on the same path fails because function files already exist.
	},
}

func (s *Server) createHandler(ctx context.Context, r *mcp.CallToolRequest, input CreateInput) (result *mcp.CallToolResult, output CreateOutput, err error) {
	out, err := s.executor.Execute(ctx, "create", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = CreateOutput{
		Runtime:  input.Language,
		Template: input.Template,
		Message:  string(out),
	}
	return
}

type CreateInput struct {
	Language   string  `json:"language" jsonschema:"required,Language runtime to use"`
	Path       string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Template   *string `json:"template,omitempty" jsonschema:"Function template (e.g., http, cloudevents)"`
	Repository *string `json:"repository,omitempty" jsonschema:"Git repository URI containing custom templates"`
	Verbose    *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i CreateInput) Args() []string {
	args := []string{"-l", i.Language, "--path", i.Path}

	// Optional
	args = appendStringFlag(args, "--template", i.Template)
	args = appendStringFlag(args, "--repository", i.Repository)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type CreateOutput struct {
	Runtime  string  `json:"runtime" jsonschema:"Language runtime used"`
	Template *string `json:"template" jsonschema:"Template used"`
	Message  string  `json:"message,omitempty" jsonschema:"Output message"`
}
