package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigEnvs_Add ensures the config envs tool executes with all arguments for add action.
func TestTool_ConfigEnvs_Add(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":  {"path", "--path", "."},
		"name":  {"name", "--name", "API_KEY"},
		"value": {"value", "--value", "secret123"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	action := "add"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args (subcommand and action), got %d: %v", len(args), args)
		}

		// Validate "envs" subcommand
		if args[0] != "envs" {
			t.Fatalf("expected args[0]='envs', got %q", args[0])
		}

		// Validate action
		if args[1] != action {
			t.Fatalf("expected args[1]=%q, got %q", action, args[1])
		}

		// Validate flags (skip first 2 args which are "envs" and "add")
		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)
	inputArgs["action"] = action

	// Invoke tool with all arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs",
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

// TestTool_ConfigEnvs_List ensures the config envs tool can list environment variables.
func TestTool_ConfigEnvs_List(t *testing.T) {
	action := "list"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// For list action, "envs" + "--path" flag = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "envs" {
			t.Fatalf("expected args[0]='envs', got %q", args[0])
		}

		// Validate path flag
		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path flag with value '.', got %q", val)
		}

		return []byte("DATABASE_URL=postgres://localhost\nAPI_KEY=secret\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs",
		Arguments: map[string]any{
			"action": action,
			"path":   ".",
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
