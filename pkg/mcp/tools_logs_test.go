package mcp

import (
	"context"
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
		"path":      {"path", "--path", "/home/user/myfunc"},
		"namespace": {"namespace", "--namespace", "prod"},
		"since":     {"since", "--since", "10m"},
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
		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)
		// stderr carries the status banner, which must not leak into the logs.
		return []byte(wantLogs), []byte("Logs for function 'myfunc' in namespace 'prod'...\n"), nil
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

// TestTool_Logs_Args_ByName ensures the logs tool passes --name when the
// Function is identified by name instead of path, and returns the log content.
func TestTool_Logs_Args_ByName(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"name": {"name", "--name", "my-function"},
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
