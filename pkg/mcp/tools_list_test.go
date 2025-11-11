package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_List_Args ensures the list tool executes with all arguments passed correctly.
func TestTool_List_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
		"output":    {"output", "--output", "json"},
	}

	boolFlags := map[string]string{
		"allNamespaces": "--all-namespaces",
		"verbose":       "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "list" {
			t.Fatalf("expected subcommand 'list', got %q", subcommand)
		}

		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)

		return []byte("NAME\tNAMESPACE\tRUNTIME\nmy-func\tprod\tgo\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)

	// Invoke tool with all optional arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
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
