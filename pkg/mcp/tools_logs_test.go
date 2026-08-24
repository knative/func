package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
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

// TestTool_Logs_NamespaceRequiresName ensures 'namespace' is rejected when
// combined with 'path'. The CLI resolves the namespace from the Function's own
// deploy identity in path mode and ignores --namespace, so accepting it would
// return another namespace's logs while reporting success.
func TestTool_Logs_NamespaceRequiresName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		t.Fatal("executor should not be called when namespace is combined with path")
		return nil, nil, nil
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
	if !result.IsError {
		t.Fatal("expected an error result when namespace is combined with path")
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
	if output.Warnings == "" {
		t.Error("expected truncation to be reported in warnings")
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
