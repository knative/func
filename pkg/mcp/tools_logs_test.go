package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Logs_Args_ByPath ensures the logs tool passes all arguments correctly
// when identifying the Function by path, and returns the executor's stdout as
// the log content.
func TestTool_Logs_Args_ByPath(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":  {"path", "--path", "/home/user/myfunc"},
		"since": {"since", "--since", "10m"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	const wantLogs = "2024/01/01 12:00:00 INFO handler invoked\n"

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		if subcommand != "logs" {
			t.Fatalf("expected subcommand 'logs', got %q", subcommand)
		}
		// '--tail 20' is a flag/value pair like the string flags, but is
		// passed as an integer in the tool's input schema, so it is checked
		// separately from the table-driven string flags.
		validateArgLength(t, args, len(stringFlags)+1, len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)
		if got := argsToMap(args)["--tail"]; got != "20" {
			t.Fatalf("expected --tail 20, got %q", got)
		}
		return []byte(wantLogs), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	args := buildInputArgs(stringFlags, boolFlags)
	args["tail"] = 20

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: args,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteSplitInvoked {
		t.Fatal("executor was not invoked")
	}

	var output LogsOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if output.Logs != wantLogs {
		t.Errorf("expected logs %q, got %q", wantLogs, output.Logs)
	}
	if output.Truncated {
		t.Error("expected output to not be marked truncated")
	}
}

// TestTool_Logs_Args_ByName ensures the logs tool passes --name when the
// Function is identified by name instead of path, that 'namespace' is
// forwarded in this mode, and that the log content is returned.
func TestTool_Logs_Args_ByName(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"name":      {"name", "--name", "my-function"},
		"namespace": {"namespace", "--namespace", "prod"},
	}

	boolFlags := map[string]string{}

	const wantLogs = "log line 1\nlog line 2\n"

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		if subcommand != "logs" {
			t.Fatalf("expected subcommand 'logs', got %q", subcommand)
		}
		// No 'tail' was provided, so the default bound is forwarded in
		// addition to the flags under test.
		validateArgLength(t, args, len(stringFlags)+1, len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		if got := argsToMap(args)["--tail"]; got != strconv.Itoa(defaultTailLines) {
			t.Fatalf("expected --tail %d, got %q", defaultTailLines, got)
		}
		return []byte(wantLogs), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: buildInputArgs(stringFlags, boolFlags),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteSplitInvoked {
		t.Fatal("executor was not invoked")
	}

	var output LogsOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if output.Logs != wantLogs {
		t.Errorf("expected logs %q, got %q", wantLogs, output.Logs)
	}
}

// TestTool_Logs_MutuallyExclusive ensures that providing both 'path' and 'name'
// returns an error rather than executing the command.
func TestTool_Logs_MutuallyExclusive(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		t.Fatal("executor should not be called when both path and name are provided")
		return nil, nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "logs",
		Arguments: map[string]any{
			"path": "/home/user/myfunc",
			"name": "my-function",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when both path and name are provided")
	}
}

// TestTool_Logs_RequiresPathOrName ensures the logs tool errors when neither
// 'path' nor 'name' is provided. Unlike the CLI, the MCP tool does not fall
// back to the server's working directory, since an agent has no meaningful cwd.
func TestTool_Logs_RequiresPathOrName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		t.Fatal("executor should not be called when neither path nor name is provided")
		return nil, nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when neither path nor name is provided")
	}
}

// TestTool_Logs_NamespaceWithPath ensures 'namespace' is forwarded even when
// combined with 'path', rather than being rejected. The CLI accepts the
// combination (it reads the namespace from the Function's own deploy identity
// and ignores the flag), so rejecting it here would make the tool enforce a
// rule 'func logs' does not.
func TestTool_Logs_NamespaceWithPath(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		argsMap := argsToMap(args)
		if got := argsMap["--path"]; got != "/home/user/myfunc" {
			t.Fatalf("expected --path /home/user/myfunc, got %q", got)
		}
		if got := argsMap["--namespace"]; got != "prod" {
			t.Fatalf("expected --namespace prod, got %q", got)
		}
		return []byte("log line\n"), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "logs",
		Arguments: map[string]any{
			"path":      "/home/user/myfunc",
			"namespace": "prod",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteSplitInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_Logs_TailUnlimited ensures an explicit negative 'tail' is passed
// through rather than being replaced by the default bound. Negative means
// unlimited in the CLI, and is the caller's only way to ask for every
// retained line.
func TestTool_Logs_TailUnlimited(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		if got := argsToMap(args)["--tail"]; got != "-1" {
			t.Fatalf("expected --tail -1, got %q", got)
		}
		return []byte("log line\n"), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{"name": "myfunc", "tail": -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

// TestTool_Logs_Warnings ensures the notices 'func logs' writes to stderr on an
// otherwise-successful call are surfaced rather than discarded, and that they
// do not leak into the log content itself. A Function scaled to zero exits
// zero with empty stdout, which is otherwise indistinguishable from a Function
// which ran and printed nothing.
func TestTool_Logs_Warnings(t *testing.T) {
	const notice = "No running or recently terminated pods found for function 'myfunc' in namespace 'prod'. It may have scaled to zero, in which case there are no logs to print."

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return nil, []byte(notice + "\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{"name": "myfunc", "namespace": "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output LogsOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if output.Logs != "" {
		t.Errorf("expected empty logs, got %q", output.Logs)
	}
	if output.Warnings != notice {
		t.Errorf("expected warnings %q, got %q", notice, output.Warnings)
	}
}

// TestTool_Logs_Truncated ensures a log payload larger than the limit is
// bounded to its most recent lines and reported as truncated, rather than
// being returned whole or silently shortened.
func TestTool_Logs_Truncated(t *testing.T) {
	const lastLine = "the most recent line\n"
	oversized := strings.Repeat("a log line of some length\n", (maxLogBytes/26)+100) + lastLine

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte(oversized), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{"name": "myfunc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output LogsOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if !output.Truncated {
		t.Error("expected output to be marked truncated")
	}
	if len(output.Logs) > maxLogBytes {
		t.Errorf("expected at most %d bytes of logs, got %d", maxLogBytes, len(output.Logs))
	}
	if !strings.HasSuffix(output.Logs, lastLine) {
		t.Error("expected the most recent lines to be retained")
	}
	if first, _, _ := strings.Cut(output.Logs, "\n"); first != "a log line of some length" {
		t.Errorf("expected truncation to a line boundary, got partial first line %q", first)
	}
	// Truncation is reported by 'truncated' alone. Repeating it in
	// 'warnings' would give the caller two channels for one fact, and
	// 'warnings' is reserved for what the CLI reported.
	if output.Warnings != "" {
		t.Errorf("expected truncation to be reported by 'truncated' only, got warnings %q", output.Warnings)
	}
}

// TestTool_Logs_WarningsBounded ensures the notices returned alongside the
// logs are themselves bounded. stderr is not guaranteed to be a line or two:
// 'verbose' writes there, as does one warning per unreadable pod, so an
// unbounded 'warnings' would defeat the cap on the payload as a whole.
func TestTool_Logs_WarningsBounded(t *testing.T) {
	const lastNotice = "Warning: the newest notice"
	oversized := strings.Repeat("Warning: a pod's logs could not be read\n", (maxWarningBytes/40)+100) + lastNotice

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte("log line\n"), []byte(oversized), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{"name": "myfunc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output LogsOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Warnings) > maxWarningBytes+len(warningsTruncatedMarker)+1 {
		t.Errorf("expected warnings bounded to ~%d bytes, got %d", maxWarningBytes, len(output.Warnings))
	}
	if !strings.HasPrefix(output.Warnings, warningsTruncatedMarker) {
		t.Error("expected dropped notices to be marked, so the remainder is not mistaken for all of them")
	}
	if !strings.HasSuffix(output.Warnings, lastNotice) {
		t.Error("expected the most recent notices to be retained")
	}
	// Bounding the notices must not be confused with bounding the logs.
	if output.Truncated {
		t.Error("expected 'truncated' to describe the logs only, not the warnings")
	}
}

// TestTruncateTail exercises the bounding directly, with a small limit so the
// boundary cases are exact. Two of them are regressions: a cut which lands on
// a line boundary must not consume the intact line which begins there, and a
// line longer than the limit must not be returned as a broken rune.
func TestTruncateTail(t *testing.T) {
	tests := []struct {
		name          string
		logs          string
		limit         int
		want          string
		wantTruncated bool
	}{
		{
			name: "under the limit is returned whole",
			logs: "aaaa\nbbbb\n", limit: 100,
			want: "aaaa\nbbbb\n", wantTruncated: false,
		},
		{
			name: "exactly at the limit is returned whole",
			logs: "aaaa\nbbbb\n", limit: 10,
			want: "aaaa\nbbbb\n", wantTruncated: false,
		},
		{
			// The cut falls immediately after "aaaa\n", so "bbbb" is a
			// whole, intact line. Searching forward for a newline from
			// there would find that line's own terminator and drop it.
			name: "cut on a line boundary keeps the line beginning there",
			logs: "aaaa\nbbbb\ncccc\n", limit: 10,
			want: "bbbb\ncccc\n", wantTruncated: true,
		},
		{
			// The cut falls inside "aaaa", whose remainder must go.
			name: "cut inside a line drops that line's remainder",
			logs: "aaaa\nbbbb\ncccc\n", limit: 12,
			want: "bbbb\ncccc\n", wantTruncated: true,
		},
		{
			name: "single line longer than the limit keeps its tail",
			logs: "abcdef", limit: 3,
			want: "def", wantTruncated: true,
		},
		{
			// Three-byte runes and no newline to re-align on: a byte-wise
			// cut lands mid-rune and would otherwise emit invalid UTF-8,
			// which JSON encoding silently replaces with U+FFFD.
			name: "single long line is trimmed to a rune boundary",
			logs: "世世世世", limit: 5,
			want: "世", wantTruncated: true,
		},
		{
			// Content which was not valid UTF-8 to begin with (binary
			// written to stdout) cannot be made valid by trimming. The
			// contract is only that this introduces no corruption of its
			// own, so such input is passed through as it was found.
			name: "input which is already invalid UTF-8 is passed through",
			logs: "\xb8\xb8\xb8\xb8", limit: 3,
			want: "\xb8\xb8\xb8", wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := truncateTail(tt.logs, tt.limit)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("expected truncated %v, got %v", tt.wantTruncated, truncated)
			}
			if len(got) > tt.limit && tt.limit > 0 {
				t.Errorf("expected at most %d bytes, got %d", tt.limit, len(got))
			}
			// Bounding must not introduce invalid UTF-8 into content which
			// was valid: JSON encoding replaces it with U+FFFD.
			if utf8.ValidString(tt.logs) && !utf8.ValidString(got) {
				t.Errorf("expected valid UTF-8, got % x", got)
			}
		})
	}
}

// TestTool_Logs_ExecutorError ensures a failed command surfaces as a tool error
// which includes both streams, since the CLI reports why it failed on stderr.
func TestTool_Logs_ExecutorError(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("function not deployed or not found"), fmt.Errorf("exit status 1")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "logs",
		Arguments: map[string]any{"name": "myfunc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when the command fails")
	}
	if msg := resultToString(result); !strings.Contains(msg, "function not deployed or not found") {
		t.Errorf("expected the command's stderr in the error, got %q", msg)
	}
}
