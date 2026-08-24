package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var logsTool = &mcp.Tool{
	Name:        "logs",
	Title:       "Get Function Logs",
	Description: "Retrieve a finite snapshot of recent logs from a deployed Function (prints and returns, does not stream). Use --since to bound the time window (e.g. '5m', '1h'). Identify the Function by path (reads func.yaml) or by name.",
	Annotations: &mcp.ToolAnnotations{
		Title:        "Get Function Logs",
		ReadOnlyHint: true,
		// A logs snapshot is read-only but not idempotent: the same call
		// made later returns a different (growing) set of log lines, so
		// IdempotentHint is intentionally left unset (false).
		DestructiveHint: ptr(false),
	},
}

func (s *Server) logsHandler(ctx context.Context, r *mcp.CallToolRequest, input LogsInput) (result *mcp.CallToolResult, output LogsOutput, err error) {
	// Unlike the CLI, the MCP tool does not fall back to the server's working
	// directory: an agent has no meaningful cwd, so require an explicit target.
	// Exactly one of Path or Name must be provided (same shape as describe/delete).
	if (input.Path != nil && input.Name != nil) || (input.Path == nil && input.Name == nil) {
		err = fmt.Errorf("exactly one of 'path' or 'name' must be provided")
		return
	}

	// ExecuteSplit (rather than Execute/CombinedOutput) is required here so the
	// returned logs are clean stdout only. `func logs` writes a status banner to
	// stderr (e.g. "Logs for function '…' in namespace '…' since '…'...") that
	// would otherwise be interleaved into the log output an agent parses.
	stdout, stderr, err := s.executor.ExecuteSplit(ctx, "logs", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\nstdout: %s\nstderr: %s", err, string(stdout), string(stderr))
		return
	}
	output = LogsOutput{
		Logs: string(stdout),
	}
	return
}

// LogsInput defines the input parameters for the logs tool.
// Exactly one of Path or Name must be provided.
type LogsInput struct {
	Path      *string `json:"path,omitempty"      jsonschema:"Absolute path to the Function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty"      jsonschema:"Name of the deployed Function to fetch logs for (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the Function (default: current namespace)"`
	Since     *string `json:"since,omitempty"     jsonschema:"Return logs newer than a relative duration such as 30s, 5m, or 2h (default: all available logs)"`
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
