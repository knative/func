package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Describe_Args ensures the describe tool executes with all arguments passed correctly
// when using a name (positional argument).
func TestTool_Describe_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
		"output":    {"output", "--output", "json"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	name := "my-function"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}

		// Expected: 1 positional + 2 string flags + 1 bool flag = 1 + 2*2 + 1 = 6 args
		if len(args) != 1+len(stringFlags)*2+len(boolFlags) {
			t.Fatalf("expected %d args, got %d: %v", 1+len(stringFlags)*2+len(boolFlags), len(args), args)
		}

		// Validate positional argument (name) comes first
		if args[0] != name {
			t.Fatalf("expected positional arg %q, got %q", name, args[0])
		}

		// Validate flags
		validateStringFlags(t, args[1:], stringFlags)
		validateBoolFlags(t, args[1:], boolFlags)

		return []byte("Function 'my-function' in namespace 'prod'\n"), nil
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

// TestTool_Describe_PathBased ensures the describe tool passes --path flag correctly.
func TestTool_Describe_PathBased(t *testing.T) {
	path := "/home/user/my-project"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}

		// Expected: --path /home/user/my-project = 2 args
		if len(args) != 2 {
			t.Fatalf("expected 2 args, got %d: %v", len(args), args)
		}
		if args[0] != "--path" || args[1] != path {
			t.Fatalf("expected '--path %s', got %v", path, args)
		}

		return []byte("Function details\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := map[string]any{
		"path": path,
	}

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

// TestTool_Describe_MutualExclusion ensures that providing both path and name returns an error.
func TestTool_Describe_MutualExclusion(t *testing.T) {
	executor := mock.NewExecutor()

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := map[string]any{
		"path": "/some/path",
		"name": "my-function",
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
		Arguments: inputArgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when both path and name are provided")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not be invoked when validation fails")
	}
}

// TestTool_Describe_NeitherProvided ensures the describe tool works with no path or name,
// describing the function in the current working directory.
func TestTool_Describe_NeitherProvided(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected 0 args, got %d: %v", len(args), args)
		}
		return []byte("Function details\n"), nil
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

// TestTool_Describe_BinaryFailure ensures that executor errors are propagated correctly.
func TestTool_Describe_BinaryFailure(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("error output"), fmt.Errorf("command failed")
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
	if !result.IsError {
		t.Fatal("expected error result when executor fails")
	}
}
