package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigLabels_Add ensures the config labels tool executes with all arguments for add action.
func TestTool_ConfigLabels_Add(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":  {"path", "--path", "."},
		"name":  {"name", "--name", "environment"},
		"value": {"value", "--value", "prod"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	// Required field
	action := "add"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args (subcommand and action), got %d: %v", len(args), args)
		}

		// Validate "labels" subcommand
		if args[0] != "labels" {
			t.Fatalf("expected args[0]='labels', got %q", args[0])
		}

		// Validate action
		if args[1] != action {
			t.Fatalf("expected args[1]=%q, got %q", action, args[1])
		}

		// Validate flags (skip first 2 args which are "labels" and "add")
		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Label added successfully\n"), nil
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
		Name:      "config_labels",
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

// TestTool_ConfigLabels_List ensures the config labels tool can list labels.
func TestTool_ConfigLabels_List(t *testing.T) {
	action := "list"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// For list action, "labels" + "--path" flag = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "labels" {
			t.Fatalf("expected args[0]='labels', got %q", args[0])
		}

		// Validate path flag
		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path flag with value '.', got %q", val)
		}

		return []byte("app=my-function\nenvironment=prod\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_labels",
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
