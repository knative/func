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
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}
		// expect: ["my-func", "--namespace", "prod"]
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "my-func" {
			t.Fatalf("expected positional name 'my-func', got %q", args[0])
		}
		argsMap := argsToMap(args[1:])
		if ns, ok := argsMap["--namespace"]; !ok || ns != "prod" {
			t.Fatalf("expected --namespace prod, got map: %v", argsMap)
		}
		return []byte("Function name:\n  my-func\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "describe",
		Arguments: map[string]any{
			"name":      "my-func",
			"namespace": "prod",
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
