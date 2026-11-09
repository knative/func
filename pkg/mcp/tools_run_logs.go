package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var runLogsTool = &mcp.Tool{
	Name:        "run_logs",
	Title:       "Get Local Function Run Output",
	Description: fmt.Sprintf("Retrieve a finite snapshot of the recent output (stdout and stderr) of a Function started with the run tool (does not stream). Only the most recent %d lines are retained. If the Function exited on its own (for example crashed), its final output is still returned until run_stop is called or a new run starts at the same path. Use it to debug a locally running Function, for example after an invoke returned an error. For a deployed Function use the logs tool instead.", tailBufferSize),
	Annotations: &mcp.ToolAnnotations{
		Title:        "Get Local Function Run Output",
		ReadOnlyHint: true,
		// Not idempotent: the same call made later returns different
		// (growing) output, so IdempotentHint is left unset (false).
	},
}

func (s *Server) runLogsHandler(ctx context.Context, r *mcp.CallToolRequest, input RunLogsInput) (result *mcp.CallToolResult, output RunLogsOutput, err error) {
	if err = validatePath(input.Path); err != nil {
		return
	}
	path, err := resolveRunPath(input.Path)
	if err != nil {
		err = fmt.Errorf("unable to resolve function path: %w", err)
		return
	}
	if input.Tail != nil && *input.Tail < 1 {
		err = fmt.Errorf("tail must be at least 1, got %d", *input.Tail)
		return
	}

	// An active run first. Failing that, a run which exited on its own is
	// still readable: its final output is what explains the crash.
	running := true
	entry, ok := s.runs.get(path)
	if !ok {
		running = false
		entry, ok = s.runs.getExited(path)
	}
	if !ok {
		// Not an error, mirroring run_stop: there is simply no run to read.
		output = RunLogsOutput{
			Message: fmt.Sprintf("no run found at %q; start one with the run tool", path),
		}
		return
	}
	if entry.output == nil {
		output = RunLogsOutput{
			Running: running,
			Message: fmt.Sprintf("output of the run at %q (pid %d) is not retained", path, entry.pid),
		}
		return
	}

	text, truncated := entry.output()
	logs, tailed := tailLines(text, input.Tail)
	// maxLogBytes is a backstop for a few very long lines; the line count
	// alone does not bound the payload.
	logs, cut := truncateTail(logs, maxLogBytes)

	output = RunLogsOutput{
		Logs:      logs,
		Truncated: truncated || tailed || cut,
		Running:   running,
	}
	if !running {
		output.Message = fmt.Sprintf("the Function at %q (pid %d) is no longer running; this is its final output", path, entry.pid)
	}
	return
}

// tailLines keeps the last n lines of s (all of them when n is nil) and
// reports whether any line was dropped.
func tailLines(s string, n *int) (string, bool) {
	if n == nil || s == "" {
		return s, false
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= *n {
		return s, false
	}
	return strings.Join(lines[len(lines)-*n:], "\n"), true
}

// RunLogsInput defines the input parameters for the run_logs tool.
type RunLogsInput struct {
	Path string `json:"path" jsonschema:"required,Absolute path to the function project directory; must match the path used with the run tool"`
	Tail *int   `json:"tail,omitempty" jsonschema:"Number of most recent output lines to return (default: every retained line; must be at least 1)"`
}

// RunLogsOutput defines the structured output returned by the run_logs tool.
type RunLogsOutput struct {
	Logs      string `json:"logs" jsonschema:"Recent output of the locally running Function; each line is prefixed with 'stdout:' or 'stderr:'"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"True if older lines were dropped, either because only the most recent lines are retained, because of 'tail', or because the output exceeded the tool's size limit. Do not assume the output is complete"`
	Running   bool   `json:"running" jsonschema:"True if the Function process is still running. False means it exited on its own (for example crashed) and the logs are its final output"`
	Message   string `json:"message,omitempty" jsonschema:"Informational message, set when there is no run to read output from or when the process has exited"`
}
