package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Describe_ByPath ensures the describe tool passes --path and optional flags correctly.
func TestTool_Describe_ByPath(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":   {"path", "--path", "./my-func"},
		"output": {"output", "--output", "json"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}
		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)
		return []byte("Function name:\n  my-func\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
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

// TestTool_Describe_ByName ensures the describe tool passes the function name as a positional argument.
func TestTool_Describe_ByName(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
	}

	boolFlags := map[string]string{}

	name := "my-func"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}

		// Expected: 1 positional + 1 string flag * 2 + 0 bool flags = 3 args
		if len(args) != 1+len(stringFlags)*2+len(boolFlags) {
			t.Fatalf("expected %d args, got %d: %v", 1+len(stringFlags)*2+len(boolFlags), len(args), args)
		}

		// Validate positional name argument comes first
		if args[0] != name {
			t.Fatalf("expected positional arg %q, got %q", name, args[0])
		}

		validateStringFlags(t, args[1:], stringFlags)
		validateBoolFlags(t, args[1:], boolFlags)

		return []byte("Function name:\n  my-func\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)
	inputArgs["name"] = name

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
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

// TestTool_Describe_NoArgs ensures the describe tool works with no arguments,
// falling back to describing the function in the current working directory.
func TestTool_Describe_NoArgs(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args for current-directory describe, got %d: %v", len(args), args)
		}
		return []byte("Function name:\n  my-func\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
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

// TestTool_Describe_PathAndNameConflict ensures an error is returned when both path and name are provided.
func TestTool_Describe_PathAndNameConflict(t *testing.T) {
	executor := mock.NewExecutor()

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "describe",
		Arguments: map[string]any{
			"path": "./my-func",
			"name": "my-func",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when both path and name are provided")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not have been invoked")
	}
}

// TestTool_Describe_NamespaceWithoutName ensures an error is returned when namespace is set without a name.
func TestTool_Describe_NamespaceWithoutName(t *testing.T) {
	executor := mock.NewExecutor()

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "describe",
		Arguments: map[string]any{
			"namespace": "prod",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when namespace is provided without name")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not have been invoked")
	}
}
