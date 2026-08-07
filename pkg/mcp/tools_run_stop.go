package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var runStopTool = &mcp.Tool{
	Name:        "run_stop",
	Title:       "Stop Local Function Run",
	Description: "Stop a Function previously started with the run tool.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Stop Local Function Run",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  true, // stopping an already-stopped run at the same path fails clearly either way
	},
}

func (s *Server) runStopHandler(ctx context.Context, r *mcp.CallToolRequest, input RunStopInput) (result *mcp.CallToolResult, output RunStopOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in read-only mode; to enable write operations, set FUNC_ENABLE_MCP_WRITE in the server environment and restart the server")
		return
	}

	path, err := resolveRunPath(input.Path)
	if err != nil {
		err = fmt.Errorf("unable to resolve function path: %w", err)
		return
	}

	entry, ok := s.runs.get(path)
	if !ok {
		err = fmt.Errorf("no running function found at %q", path)
		return
	}

	if err = entry.stop(); err != nil {
		err = fmt.Errorf("unable to stop function at %q: %w", path, err)
		return
	}
	s.runs.remove(path)

	output = RunStopOutput{
		Message: fmt.Sprintf("stopped function at %q (pid %d)", path, entry.pid),
	}
	return
}

// RunStopInput defines the input parameters for the run_stop tool.
type RunStopInput struct {
	Path *string `json:"path,omitempty" jsonschema:"Absolute path to the function project directory (default: server's current working directory); must match the path used with the run tool"`
}

// RunStopOutput defines the structured output returned by the run_stop tool.
type RunStopOutput struct {
	Message string `json:"message" jsonschema:"Confirmation message"`
}
