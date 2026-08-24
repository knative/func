package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultTailLines bounds at the source how much log output the CLI is asked
// to gather when the caller has not bounded it themselves.  'func logs'
// defaults to every line the Function's pods have retained, and the executor
// buffers that in full before this package sees any of it, so a default is
// applied here rather than gathering everything and discarding most of it
// after the fact.  It is declared in the tool's input schema, and a negative
// 'tail' still means unlimited, exactly as it does in the CLI.
const defaultTailLines = 1000

// maxLogBytes is a backstop on the log payload returned to the caller, for
// the cases defaultTailLines does not bound: an explicitly large (or
// unlimited) 'tail', or a Function served by many pods, each contributing its
// own tail.  The most recent lines are kept, and the fact that older lines
// were dropped is reported via LogsOutput.Truncated rather than being silent.
const maxLogBytes = 256 * 1024

// maxWarningBytes bounds the notices returned alongside the logs.  stderr is
// not necessarily a line or two: 'verbose' writes its output there, as does
// one warning per pod whose logs could not be read, so it needs a bound of
// its own rather than being returned whole.
const maxWarningBytes = 8 * 1024

// warningsTruncatedMarker introduces a warnings payload which was itself too
// large to return, so that the notices which remain are not mistaken for all
// of them.
const warningsTruncatedMarker = "[...older notices omitted...]"

var logsTool = &mcp.Tool{
	Name:        "logs",
	Title:       "Get Function Logs",
	Description: "Retrieve a finite snapshot of recent logs from a deployed Function (prints and returns, does not stream). Returns the most recent 1000 lines per pod by default; bound it explicitly with 'tail' and/or 'since' (time window, e.g. '5m'). Identify the Function by path (reads func.yaml) or by name.",
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
	//
	// NOTE: no further validation is performed here.  In particular
	// 'namespace' is forwarded as given, including alongside 'path', where
	// the CLI reads the namespace from the Function's own deploy identity and
	// ignores the flag.  Rejecting that combination would be a rule this tool
	// enforces and 'func logs' does not, which is a divergence between the two
	// surfaces rather than a fix.
	if (input.Path != nil && input.Name != nil) || (input.Path == nil && input.Name == nil) {
		err = fmt.Errorf("exactly one of 'path' or 'name' must be provided")
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

	// Truncation of the logs is reported by Truncated alone.  It is
	// deliberately not also described in Warnings: two channels for one fact
	// invite the caller to report it twice, and Warnings is what the CLI
	// said, not what this tool did to the payload.
	logs, truncated := truncateTail(string(stdout), maxLogBytes)

	warnings, warningsTruncated := truncateTail(strings.TrimSpace(string(stderr)), maxWarningBytes)
	if warningsTruncated {
		warnings = warningsTruncatedMarker + "\n" + warnings
	}

	output = LogsOutput{
		Logs:      logs,
		Truncated: truncated,
		Warnings:  warnings,
	}
	return
}

// truncateTail bounds s to its last 'limit' bytes, keeping the most recent lines
// and reporting whether anything was dropped.
//
// Whole lines are preserved in both directions: the first line returned is
// never a fragment of an older one, and no complete line is discarded in
// order to reach a line boundary the cut had already landed on.  A single
// line longer than the limit is the one case which cannot be honored; its tail is
// returned, trimmed to a rune boundary so that the result is still valid
// UTF-8.
func truncateTail(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	cut := len(s) - limit // >= 1, since len(s) > limit

	// A cut which lands immediately after a newline already begins a whole,
	// intact line.  Searching forward for a newline from here would find that
	// line's own terminator and silently drop the entire line.
	if s[cut-1] == '\n' {
		return s[cut:], true
	}

	// Otherwise the cut landed inside a line; drop that line's remainder.
	if i := strings.IndexByte(s[cut:], '\n'); i >= 0 {
		return s[cut+i+1:], true
	}

	// No newline anywhere in the retained window: a single line longer than
	// the limit, such as one JSON log record or an un-terminated stack trace.
	return trimPartialRune(s[cut:]), true
}

// trimPartialRune drops the leading bytes of a UTF-8 sequence which a
// byte-wise cut left incomplete, so that s begins on a rune boundary.
// Without it, cutting multi-byte content mid-rune yields invalid UTF-8, which
// JSON encoding replaces with U+FFFD.
func trimPartialRune(s string) string {
	for i := 0; i < len(s) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(s[i]) {
			return s[i:]
		}
	}
	return s
}

// LogsInput defines the input parameters for the logs tool.
// Exactly one of Path or Name must be provided.
type LogsInput struct {
	Path      *string `json:"path,omitempty"      jsonschema:"Absolute path to the Function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty"      jsonschema:"Name of the deployed Function to fetch logs for (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the Function (default: current namespace). Applies when identifying the Function by 'name'; in 'path' mode the namespace is read from the Function's own deploy identity in func.yaml and this has no effect, as in the CLI"`
	Since     *string `json:"since,omitempty"     jsonschema:"Return logs newer than a relative duration such as 30s, 5m, or 2h (default: all retained logs, subject to tail)"`
	Tail      *int    `json:"tail,omitempty"      jsonschema:"Number of most recent log lines to return per pod (default: 1000; a negative value means unlimited, as in the CLI). Lower it, or add 'since', to bound a large or unknown log volume further"`
	Verbose   *bool   `json:"verbose,omitempty"   jsonschema:"Enable verbose logging output. Note this is written to the same stream as the notices returned in 'warnings'"`
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

	// Always bounded at the source: the caller's value when given, otherwise
	// the default.  See defaultTailLines.
	tail := defaultTailLines
	if i.Tail != nil {
		tail = *i.Tail
	}
	args = append(args, "--tail", strconv.Itoa(tail))

	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

// LogsOutput defines the structured output returned by the logs tool.
type LogsOutput struct {
	Logs      string `json:"logs" jsonschema:"Log output from the deployed Function. When more than one pod is serving the Function, each line is prefixed with the pod it came from"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"True if older lines were dropped because the output exceeded the tool's size limit; only the most recent lines are included. Re-run with a smaller 'tail' or a shorter 'since' window rather than assuming the output is complete"`
	Warnings  string `json:"warnings,omitempty" jsonschema:"Non-fatal notices the CLI emitted while gathering the logs. Empty or partial output is explained here: that the Function has scaled to zero and so has no logs to print, or that the logs of some of its pods could not be read. Always check this before concluding that a Function produced no output"`
}
