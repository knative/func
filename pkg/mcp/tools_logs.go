package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxLogBytes bounds the log payload returned by the logs tool.  A snapshot
// defaults to every line the Function's pods have retained (see 'func logs
// --tail'), which for a chatty Function is more than an agent's context can
// hold.  The most recent lines are kept, and the fact that older lines were
// dropped is reported via LogsOutput.Truncated rather than being silent.
const maxLogBytes = 256 * 1024

var logsTool = &mcp.Tool{
	Name:        "logs",
	Title:       "Get Function Logs",
	Description: "Retrieve a finite snapshot of recent logs from a deployed Function (prints and returns, does not stream). Bound the output with 'since' (time window, e.g. '5m') and 'tail' (most recent lines per pod). Identify the Function by path (reads func.yaml) or by name.",
	Annotations: &mcp.ToolAnnotations{
		Title:        "Get Function Logs",
		ReadOnlyHint: true,
		// A logs snapshot is read-only but not idempotent: the same call
		// made later returns a different (growing) set of log lines, so
		// IdempotentHint is intentionally left unset (false).
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

	// Validate: namespace only makes sense alongside 'name'.  When fetching
	// logs by 'path', the CLI resolves the Function's name and namespace from
	// its own deploy identity (func.yaml) and ignores --namespace entirely.
	// Rejecting the combination is the same rule the describe tool applies,
	// and avoids returning another namespace's logs while reporting success.
	if input.Path != nil && input.Namespace != nil {
		err = fmt.Errorf("'namespace' is only valid with 'name'; when fetching logs by 'path', the namespace is read from the Function's own deploy identity")
		return
	}

	// ExecuteSplit (rather than Execute/CombinedOutput) is required here so
	// that the log content is clean stdout only.  `func logs` uses stderr for
	// everything which is not a log line, including notices which accompany a
	// successful, zero-exit call: that the Function has scaled to zero and so
	// has no logs, and that the logs of some (but not all) of its pods could
	// be read.  Those are surfaced separately as Warnings, since an empty or
	// partial payload is otherwise indistinguishable from a complete one.
	stdout, stderr, err := s.executor.ExecuteSplit(ctx, "logs", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\nstdout: %s\nstderr: %s", err, string(stdout), string(stderr))
		return
	}

	logs, truncated := truncateLogs(string(stdout))

	warnings := strings.TrimSpace(string(stderr))
	if truncated {
		notice := fmt.Sprintf("Output exceeded %d bytes: only the most recent lines are included. Use 'tail' or 'since' to bound the window.", maxLogBytes)
		if warnings == "" {
			warnings = notice
		} else {
			warnings += "\n" + notice
		}
	}

	output = LogsOutput{
		Logs:      logs,
		Truncated: truncated,
		Warnings:  warnings,
	}
	return
}

// truncateLogs bounds logs to maxLogBytes, keeping the most recent lines and
// reporting whether anything was dropped.  Truncation is to a line boundary so
// the first line returned is not a fragment of an older one.
func truncateLogs(logs string) (string, bool) {
	if len(logs) <= maxLogBytes {
		return logs, false
	}
	truncated := logs[len(logs)-maxLogBytes:]
	if i := strings.IndexByte(truncated, '\n'); i >= 0 {
		truncated = truncated[i+1:]
	}
	return truncated, true
}

// LogsInput defines the input parameters for the logs tool.
// Exactly one of Path or Name must be provided.
type LogsInput struct {
	Path      *string `json:"path,omitempty"      jsonschema:"Absolute path to the Function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty"      jsonschema:"Name of the deployed Function to fetch logs for (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the Function (default: current namespace). Only valid together with 'name'; when fetching logs by 'path' the namespace is read from the Function's own deploy identity"`
	Since     *string `json:"since,omitempty"     jsonschema:"Return logs newer than a relative duration such as 30s, 5m, or 2h (default: all available logs)"`
	Tail      *int    `json:"tail,omitempty"      jsonschema:"Number of most recent log lines to return per pod (default: unlimited). Prefer bounding large or unknown log volumes with this rather than fetching everything"`
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
	if i.Tail != nil {
		args = append(args, "--tail", fmt.Sprintf("%d", *i.Tail))
	}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// LogsOutput defines the structured output returned by the logs tool.
type LogsOutput struct {
	Logs      string `json:"logs" jsonschema:"Log output from the deployed Function. When more than one pod is serving the Function, each line is prefixed with the pod it came from"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"True if older lines were dropped because the output exceeded the size limit; only the most recent lines are included"`
	Warnings  string `json:"warnings,omitempty" jsonschema:"Non-fatal notices emitted while gathering the logs. Empty or partial output is explained here: that the Function has scaled to zero and so has no logs to print, or that the logs of some of its pods could not be read. Always check this before concluding that a Function produced no output"`
}
