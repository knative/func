package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
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
	if s.clientProvider != nil {
		client := s.clientProvider()
		if client == nil {
			err = fmt.Errorf("create tool: client provider returned nil")
			return
		}

		// Derive function name and absolute path
		derivedName, absolutePath, errVal := deriveNameAndPath(input.Path)
		if errVal != nil {
			err = errVal
			return
		}

		templateVal := fn.DefaultTemplate
		if input.Template != nil {
			templateVal = *input.Template
		}

		var customClient *fn.Client
		if input.Repository != nil && *input.Repository != "" {
			// Construct a custom client with the provided repository
			customClient = fn.New(
				fn.WithVerbose(input.Verbose != nil && *input.Verbose),
				fn.WithRepository(*input.Repository),
			)
		} else {
			customClient = client
		}

		// Direct call to pkg/functions client.Init
		_, err = customClient.Init(fn.Function{
			Name:     derivedName,
			Root:     absolutePath,
			Runtime:  input.Language,
			Template: templateVal,
		})
		if err != nil {
			return
		}

		output = CreateOutput{
			Runtime:  input.Language,
			Template: &templateVal,
			Message:  fmt.Sprintf("Created %s function in %s", input.Language, absolutePath),
		}
		return
	}

	// Fallback to executor for backward compatibility and testing
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

// deriveNameAndPath returns resolved function name and absolute path
// to the function project root.
func deriveNameAndPath(path string) (string, string, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return "", "", err
		}
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}

	return filepath.Base(absPath), absPath, nil
}
