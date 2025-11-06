package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Create_Args ensures the create tool executes, passing in args.
func TestTool_Create_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	// Note: language (-l) is required and handled separately
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":       {"path", "--path", "."},
		"template":   {"template", "--template", "cloudevents"},
		"repository": {"repository", "--repository", "https://example.com/repo"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	// Required fields
	language := "go"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "create" {
			t.Fatalf("expected subcommand 'create', got %q", subcommand)
		}

		// Expected: 1 required string flag (-l) + stringFlags + boolFlags
		if len(args) != (1+len(stringFlags))*2+len(boolFlags) {
			t.Fatalf("expected %d args, got %d: %v", (1+len(stringFlags))*2+len(boolFlags), len(args), args)
		}

		// Validate required -l flag
		argsMap := argsToMap(args)
		if val, ok := argsMap["-l"]; !ok {
			t.Fatalf("missing required flag '-l'")
		} else if val != language {
			t.Fatalf("flag '-l': expected value %q, got %q", language, val)
		}

		// Validate optional string and boolean flags
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)

		return []byte("OK\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)
	inputArgs["language"] = language

	// Invoke tool with all arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "create",
		Arguments: inputArgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(err)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestCreate_PathValidation is removed - path validation no longer exists
// Create now operates in current working directory

// TestCreate_BinaryFailure ensures errors from the func binary are returned as MCP errors
func TestTool_Create_BinaryFailure(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		// Simulate binary returning an error
		return []byte("Error: example error\n"), fmt.Errorf("exit status 1")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	// Invoke, expecting an error
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "create",
		Arguments: map[string]any{
			"language": "go",
			"path":     ".",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should return error from binary
	if !result.IsError {
		t.Fatal("expected error result when binary fails")
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor should have been invoked")
	}

	// Error should include binary output
	if !strings.Contains(resultToString(result), "example error") {
		t.Errorf("expected error to include binary output, got: %s", resultToString(result))
	}
}
