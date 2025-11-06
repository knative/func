package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigVolumes_Add ensures the config volumes tool executes with all arguments for add action.
func TestTool_ConfigVolumes_Add(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":      {"path", "--path", "."},
		"type":      {"type", "--type", "secret"},
		"mountPath": {"mountPath", "--mount-path", "/workspace/secret"},
		"source":    {"source", "--source", "my-secret"},
		"medium":    {"medium", "--medium", "Memory"},
		"size":      {"size", "--size", "1Gi"},
	}

	boolFlags := map[string]string{
		"readOnly": "--read-only",
		"verbose":  "--verbose",
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

		// Validate "volumes" subcommand
		if args[0] != "volumes" {
			t.Fatalf("expected args[0]='volumes', got %q", args[0])
		}

		// Validate action
		if args[1] != action {
			t.Fatalf("expected args[1]=%q, got %q", action, args[1])
		}

		// Validate flags (skip first 2 args which are "volumes" and "add")
		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Volume added successfully\n"), nil
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
		Name:      "config_volumes",
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

// TestTool_ConfigVolumes_List ensures the config volumes tool can list volumes.
func TestTool_ConfigVolumes_List(t *testing.T) {
	action := "list"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// For list action, "volumes" + "--path" flag = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "volumes" {
			t.Fatalf("expected args[0]='volumes', got %q", args[0])
		}

		// Validate path flag
		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path flag with value '.', got %q", val)
		}

		return []byte("secret:my-secret:/workspace/secret\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_volumes",
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
