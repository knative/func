package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var logsTool = &mcp.Tool{
	Name:        "logs",
	Title:       "Get Function Logs",
	Description: "Retrieve recent logs from a deployed Function. Use --since to control the time window (e.g. '5m', '1h'). Identify the Function by path (reads func.yaml) or by name.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Get Function Logs",
		ReadOnlyHint:    true,
		DestructiveHint: ptr(false),
		IdempotentHint:  true,
	},
}

func (s *Server) logsHandler(ctx context.Context, r *mcp.CallToolRequest, input LogsInput) (result *mcp.CallToolResult, output LogsOutput, err error) {
	if input.Path != nil && input.Name != nil {
		err = fmt.Errorf("'path' and 'name' are mutually exclusive: provide exactly one")
		return
	}

	out, err := s.executor.Execute(ctx, "logs", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = LogsOutput{
		Logs: string(out),
	}
	return
}

// LogsInput defines the input parameters for the logs tool.
// At most one of Path or Name may be provided; if neither is given the
// server's working directory is used (same default behaviour as the CLI).
type LogsInput struct {
	Path      *string `json:"path,omitempty"      jsonschema:"Absolute path to the Function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty"      jsonschema:"Name of the deployed Function to fetch logs for (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the Function (default: current namespace)"`
	Since     *string `json:"since,omitempty"     jsonschema:"Return logs newer than a relative duration such as 30s, 5m, or 2h (default: 1m)"`
	Verbose   *bool   `json:"verbose,omitempty"   jsonschema:"Enable verbose logging output"`
}

func (i LogsInput) Args() []string {
	args := []string{}

	if i.Path != nil {
		args = append(args, "--path", *i.Path)
	} else if i.Name != nil {
		args = append(args, "--name", *i.Name)
	}

	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendStringFlag(args, "--since", i.Since)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// LogsOutput defines the structured output returned by the logs tool.
type LogsOutput struct {
	Logs string `json:"logs" jsonschema:"Log output from the deployed Function"`
}
