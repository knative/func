package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var invokeTool = &mcp.Tool{
	Name:        "invoke",
	Title:       "Invoke Function",
	Description: "Invoke a local or remote Function with test data.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Invoke Function",
		ReadOnlyHint:   false,
		IdempotentHint: false,
	},
}

func (s *Server) invokeHandler(ctx context.Context, r *mcp.CallToolRequest, input InvokeInput) (result *mcp.CallToolResult, output InvokeOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Invoke(ctx, input)
	return
}

type InvokeInput struct {
	Path        string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Target      *string `json:"target,omitempty" jsonschema:"Invocation target: local, remote, or a URL"`
	Data        *string `json:"data,omitempty" jsonschema:"Request body data"`
	ContentType *string `json:"contentType,omitempty" jsonschema:"MIME type of the data"`
	Source      *string `json:"source,omitempty" jsonschema:"CloudEvent source"`
	Type        *string `json:"type,omitempty" jsonschema:"CloudEvent type"`
	Format      *string `json:"format,omitempty" jsonschema:"Message format: http or cloudevent"`
	RequestType *string `json:"requestType,omitempty" jsonschema:"HTTP method: GET or POST"`
	Verbose     *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type InvokeOutput struct {
	Body     string              `json:"body,omitempty" jsonschema:"Response body"`
	Metadata map[string][]string `json:"metadata,omitempty" jsonschema:"Response metadata (e.g. HTTP headers)"`
	Message  string              `json:"message" jsonschema:"Status message"`
}
