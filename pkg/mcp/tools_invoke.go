package mcp

import (
	"context"
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var invokeTool = &mcp.Tool{
	Name:        "invoke",
	Title:       "Invoke Function",
	Description: "Invoke a local or remote Function with a test request.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Invoke Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // Invoking a Function may trigger arbitrary, unrepeatable side effects in the Function's handler (e.g. sending an email, charging a payment).
		IdempotentHint:  false,     // Invoking a Function may trigger arbitrary side effects in the Function's handler.
	},
}

func (s *Server) invokeHandler(ctx context.Context, r *mcp.CallToolRequest, input InvokeInput) (result *mcp.CallToolResult, output InvokeOutput, err error) {
	// Note: invoke is intentionally allowed in readonly mode. Readonly mode
	// only disables operations that mutate cluster state (deploy, delete,
	// build); invoking a Function does not itself modify the cluster.
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
type InvokeInput struct {
	Path        string            `json:"path" jsonschema:"required,Path to the function project directory"`
	Target      *string           `json:"target,omitempty" jsonschema:"Function instance to invoke: local, remote, or a URL (default: auto-discovery; prefers local when both local and remote are running)"`
	Format      *string           `json:"format,omitempty" jsonschema:"Format of message to send: http or cloudevent (default: auto-detected)"`
	ID          *string           `json:"id,omitempty" jsonschema:"CloudEvent id for the request data (default: a randomly generated UUID)"`
	Source      *string           `json:"source,omitempty" jsonschema:"CloudEvent source for the request data (default: '/boson/fn')"`
	Type        *string           `json:"type,omitempty" jsonschema:"CloudEvent type for the request data (default: 'boson.fn')"`
	Data        *string           `json:"data,omitempty" jsonschema:"Data (content) to send in the request. If both this and 'file' are omitted, a default payload of '{\"message\":\"Hello World\"}' is sent"`
	ContentType *string           `json:"contentType,omitempty" jsonschema:"MIME type of the data (default: 'application/json')"`
	RequestType *string           `json:"requestType,omitempty" jsonschema:"HTTP method: GET or POST (default: POST)"`
	File        *string           `json:"file,omitempty" jsonschema:"Path to a file whose content is used as the request data (overrides data). MUST be an absolute path: it is resolved relative to the MCP server's process working directory, not the caller's or the 'path' parameter's directory"`
	Extensions  map[string]string `json:"extensions,omitempty" jsonschema:"CloudEvent extension attributes as key-value pairs (cloudevent format only)"`
	Insecure    *bool             `json:"insecure,omitempty" jsonschema:"Skip TLS verification when invoking over SSL"`
	Verbose     *bool             `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i InvokeInput) Args() []string {
	args := []string{"--path", i.Path}

	args = appendStringFlag(args, "--target", i.Target)
	args = appendStringFlag(args, "--format", i.Format)
	args = appendStringFlag(args, "--id", i.ID)
	args = appendStringFlag(args, "--source", i.Source)
	args = appendStringFlag(args, "--type", i.Type)
	args = appendStringFlag(args, "--data", i.Data)
	args = appendStringFlag(args, "--content-type", i.ContentType)
	args = appendStringFlag(args, "--request-type", i.RequestType)
	args = appendStringFlag(args, "--file", i.File)

	if len(i.Extensions) > 0 {
		keys := make([]string, 0, len(i.Extensions))
		for k := range i.Extensions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--extension", fmt.Sprintf("%s=%s", k, i.Extensions[k]))
		}
	}

	args = appendBoolFlag(args, "--insecure", i.Insecure)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	return args
}

// InvokeOutput defines the structured output returned by the invoke tool.
type InvokeOutput struct {
	Message string `json:"message" jsonschema:"Output message, including the response body from the invoked Function"`
}
