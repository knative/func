package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Invoke_Args ensures the invoke tool executes with all arguments passed correctly.
func TestTool_Invoke_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":        {"path", "--path", "/tmp/my-func"},
		"target":      {"target", "--target", "remote"},
		"format":      {"format", "--format", "cloudevent"},
		"id":          {"id", "--id", "test-id-123"},
		"source":      {"source", "--source", "/my/source"},
		"type":        {"type", "--type", "my.event.type"},
		"data":        {"data", "--data", `{"key":"value"}`},
		"contentType": {"contentType", "--content-type", "application/json"},
		"requestType": {"requestType", "--request-type", "POST"},
		"file":        {"file", "--file", "/tmp/payload.json"},
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

		return []byte("HTTP/1.1 200 OK\nHello World\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

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

// TestTool_Invoke_NoArgs ensures invoke works with no arguments (cwd-based invocation).
func TestTool_Invoke_NoArgs(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "invoke" {
			t.Fatalf("expected subcommand 'invoke', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected 0 args for no-args invoke, got %d: %v", len(args), args)
		}
		return []byte("HTTP/1.1 200 OK\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "invoke",
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

// TestTool_Invoke_RemoteTarget ensures the target flag is passed correctly for remote invocation.
func TestTool_Invoke_RemoteTarget(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "invoke" {
			t.Fatalf("expected subcommand 'invoke', got %q", subcommand)
		}

		argsMap := argsToMap(args)
		if val, ok := argsMap["--target"]; !ok {
			t.Fatal("missing --target flag")
		} else if val != "remote" {
			t.Fatalf("expected --target 'remote', got %q", val)
		}

		return []byte("HTTP/1.1 200 OK\nResponse from remote\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "invoke",
		Arguments: map[string]any{
			"target": "remote",
		},
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

// TestTool_Invoke_BinaryFailure ensures errors from the func binary are returned as MCP errors.
func TestTool_Invoke_BinaryFailure(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("Error: function not found\n"), fmt.Errorf("exit status 1")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "invoke",
		Arguments: map[string]any{
			"target": "remote",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when binary fails")
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor should have been invoked")
	}
	if !strings.Contains(resultToString(result), "function not found") {
		t.Errorf("expected error to include binary output, got: %s", resultToString(result))
	}
}
