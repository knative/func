package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Logs_Args_ByPath ensures the logs tool passes all arguments correctly
// when identifying the Function by path.
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

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "logs" {
			t.Fatalf("expected subcommand 'logs', got %q", subcommand)
		}
		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)
		return []byte("2024/01/01 12:00:00 INFO handler invoked\n"), nil
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
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_Logs_Args_ByName ensures the logs tool passes --name when the
// Function is identified by name instead of path.
func TestTool_Logs_Args_ByName(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"name": {"name", "--name", "my-function"},
	}

	boolFlags := map[string]string{}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "logs" {
			t.Fatalf("expected subcommand 'logs', got %q", subcommand)
		}
		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		return []byte("log line 1\nlog line 2\n"), nil
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
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_Logs_MutuallyExclusive ensures that providing both 'path' and 'name'
// returns an error rather than executing the command.
func TestTool_Logs_MutuallyExclusive(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor should not be called when both path and name are provided")
		return nil, nil
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

// TestTool_Logs_NoArgs ensures the logs tool works without any arguments,
// falling back to the server's working directory (CLI default behaviour).
func TestTool_Logs_NoArgs(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "logs" {
			t.Fatalf("expected subcommand 'logs', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args when nothing is provided, got %v", args)
		}
		return []byte("no logs yet\n"), nil
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
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}
