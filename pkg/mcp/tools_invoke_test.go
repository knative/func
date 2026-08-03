package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Invoke_Args ensures the invoke tool executes with all arguments passed correctly.
func TestTool_Invoke_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":        {"path", "--path", "."},
		"target":      {"target", "--target", "remote"},
		"format":      {"format", "--format", "http"},
		"id":          {"id", "--id", "test-id"},
		"source":      {"source", "--source", "test-source"},
		"type":        {"type", "--type", "test-type"},
		"data":        {"data", "--data", "hello world"},
		"contentType": {"contentType", "--content-type", "text/plain"},
		"requestType": {"requestType", "--request-type", "GET"},
		"file":        {"file", "--file", "example.jpeg"},
	}

	boolFlags := map[string]string{
		"insecure": "--insecure",
		"verbose":  "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "invoke" {
			t.Fatalf("expected subcommand 'invoke', got %q", subcommand)
		}

		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)

		return []byte("Received: 200 OK\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)

	// Invoke tool with all optional arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
		Arguments: inputArgs,
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

// TestTool_Invoke_MinimalArgs ensures the invoke tool can be called with only
// the required 'path' argument, relying on defaults for everything else
// (target auto-discovers between local and remote).
func TestTool_Invoke_MinimalArgs(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "invoke" {
			t.Fatalf("expected subcommand 'invoke', got %q", subcommand)
		}
		want := []string{"--path", "."}
		if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
			t.Fatalf("expected args %v, got %v", want, args)
		}
		return []byte("OK\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
		Arguments: map[string]any{"path": "."},
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

// TestTool_Invoke_MissingPath ensures the invoke tool rejects a call that
// omits the required 'path' argument, since the MCP server's own working
// directory is unrelated to the Function being tested.
func TestTool_Invoke_MissingPath(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor should not be invoked when 'path' is missing")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when 'path' argument is missing")
	}
}

// TestTool_Invoke_Readonly ensures the invoke tool rejects requests in
// readonly mode, since invoking a Function may trigger arbitrary side
// effects in its handler.
func TestTool_Invoke_Readonly(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true) // readonly = true
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected invoke to be rejected in readonly mode")
	}
}

// TestTool_Invoke_Error ensures a failing invocation (e.g. non-2xx response)
// is surfaced as a tool error, allowing agents to detect invocation failures.
func TestTool_Invoke_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte(""), errors.New("failure invoking function (HTTP 500)")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected invoke failure to be reported as a tool error")
	}
}
