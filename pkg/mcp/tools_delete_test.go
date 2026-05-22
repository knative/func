package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Delete_Readonly ensures the delete tool returns the readonly-mode
// error when the server is in readonly mode.
func TestTool_Delete_Readonly(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "delete",
		Arguments: map[string]any{"name": "my-function"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result for readonly server, got success")
	}
	got := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(got, "readonly mode") {
		t.Fatalf("expected readonly-mode error, got: %q", got)
	}
}

// TestTool_Delete_BothPathAndName ensures the delete tool returns a mutual-
// exclusion validation error when both path and name are provided.
func TestTool_Delete_BothPathAndName(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "delete",
		Arguments: map[string]any{
			"path": "/some/path",
			"name": "my-function",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when both path and name are provided, got success")
	}
	got := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(got, "exactly one of 'path' or 'name'") {
		t.Fatalf("expected mutual-exclusion validation error, got: %q", got)
	}
}

// TestTool_Delete_NeitherPathNorName ensures the delete tool returns a mutual-
// exclusion validation error when neither path nor name is provided.
func TestTool_Delete_NeitherPathNorName(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "delete",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when neither path nor name is provided, got success")
	}
	got := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(got, "exactly one of 'path' or 'name'") {
		t.Fatalf("expected mutual-exclusion validation error, got: %q", got)
	}
}

// TestTool_Delete_Args ensures the delete tool executes with all arguments passed correctly.
func TestTool_Delete_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
		"all":     "--all",
	}

	// Required fields and positional arguments
	name := "my-function"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "delete" {
			t.Fatalf("expected subcommand 'delete', got %q", subcommand)
		}

		// Expected: 1 positional + 2 string flags + 1 bool flag = 1 + 2*2 + 1 = 6 args
		if len(args) != 1+len(stringFlags)*2+len(boolFlags) {
			t.Fatalf("expected %d args, got %d: %v", 1+len(stringFlags)*2+len(boolFlags), len(args), args)
		}

		// Validate positional argument (name) comes first
		if args[0] != name {
			t.Fatalf("expected positional arg %q, got %q", name, args[0])
		}

		// Validate flags (excluding positional argument at beginning)
		validateStringFlags(t, args[1:], stringFlags)
		validateBoolFlags(t, args[1:], boolFlags)

		return []byte("Function 'my-function' deleted from namespace 'prod'\n"), nil
	}

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	// Delete requires write mode - enable it for this test
	server.readonly.Store(false)

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)
	inputArgs["name"] = name

	// Invoke tool with all arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "delete",
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
