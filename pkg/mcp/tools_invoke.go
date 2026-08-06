package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var invokeTool = &mcp.Tool{
	Name:        "invoke",
	Title:       "Invoke Function",
	Description: "Invoke a deployed Function to test and validate it is working correctly. Sends an HTTP request or CloudEvent to the Function and returns the response. Use this after deploying to verify the Function handles requests as expected.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Invoke Function",
		ReadOnlyHint:   false, // Invoking a function can have side effects within the function itself.
		IdempotentHint: false,
	},
}

func (s *Server) invokeHandler(ctx context.Context, r *mcp.CallToolRequest, input InvokeInput) (result *mcp.CallToolResult, output InvokeOutput, err error) {
	out, err := s.executor.Execute(ctx, "invoke", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = InvokeOutput{
		Message: string(out),
	}
	return
}

// InvokeInput defines the input parameters for the invoke tool.
// All fields are optional since invoke can work with no arguments,
// using the current working directory and auto-discovering the target.
type InvokeInput struct {
	Path        *string `json:"path,omitempty" jsonschema:"Path to the function project directory (defaults to current directory)"`
	Target      *string `json:"target,omitempty" jsonschema:"Target to invoke: 'local' for a locally-running instance, 'remote' for the cluster deployment, or a direct URL"`
	Format      *string `json:"format,omitempty" jsonschema:"Request format: 'http' for plain HTTP request or 'cloudevent' for CloudEvents format"`
	ID          *string `json:"id,omitempty" jsonschema:"Request ID for correlation (used in CloudEvents as the event ID)"`
	Source      *string `json:"source,omitempty" jsonschema:"Request source identifier (used in CloudEvents as the event source)"`
	Type        *string `json:"type,omitempty" jsonschema:"Request type (used in CloudEvents as the event type)"`
	Data        *string `json:"data,omitempty" jsonschema:"Request data/body to send to the Function"`
	ContentType *string `json:"contentType,omitempty" jsonschema:"MIME type of the request data (e.g., application/json, text/plain)"`
	RequestType *string `json:"requestType,omitempty" jsonschema:"HTTP method to use: 'GET' or 'POST'"`
	File        *string `json:"file,omitempty" jsonschema:"Path to a file whose contents will be used as request data"`
	Insecure    *bool   `json:"insecure,omitempty" jsonschema:"Allow insecure connections (skip TLS certificate verification)"`
	Verbose     *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i InvokeInput) Args() []string {
	var args []string

	args = appendStringFlag(args, "--path", i.Path)
	args = appendStringFlag(args, "--target", i.Target)
	args = appendStringFlag(args, "--format", i.Format)
	args = appendStringFlag(args, "--id", i.ID)
	args = appendStringFlag(args, "--source", i.Source)
	args = appendStringFlag(args, "--type", i.Type)
	args = appendStringFlag(args, "--data", i.Data)
	args = appendStringFlag(args, "--content-type", i.ContentType)
	args = appendStringFlag(args, "--request-type", i.RequestType)
	args = appendStringFlag(args, "--file", i.File)

	args = appendBoolFlag(args, "--insecure", i.Insecure)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	return args
}

// InvokeOutput defines the structured output returned by the invoke tool.
type InvokeOutput struct {
	Message string `json:"message" jsonschema:"Function response output"`
}
