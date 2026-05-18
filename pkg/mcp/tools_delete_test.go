package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mcp/mock"
)

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

// TestTool_Delete_Readonly ensures the delete tool rejects requests in readonly mode.
func TestTool_Delete_Readonly(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true) // readonly = true
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
		t.Fatal("expected delete to be rejected in readonly mode")
	}
}

// TestTool_Delete_DirectClient validates direct pkg/functions client invocation for delete
func TestTool_Delete_DirectClient(t *testing.T) {
	tempDir := t.TempDir()

	fnClient := fn.New()
	
	// Create a dummy function first so there is something initialized to delete by path
	_, err := fnClient.Init(fn.Function{
		Name:     "my-func",
		Root:     tempDir,
		Runtime:  "go",
		Template: "http",
	})
	if err != nil {
		t.Fatal(err)
	}

	client, server, err := newTestPair(t, WithClientProvider(func() *fn.Client {
		return fnClient
	}))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	// Call delete with path
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "delete",
		Arguments: map[string]any{
			"path": tempDir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", resultToString(result))
	}

	// Call delete with name
	result, err = client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "delete",
		Arguments: map[string]any{
			"name":      "my-func",
			"namespace": "default",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", resultToString(result))
	}
}
