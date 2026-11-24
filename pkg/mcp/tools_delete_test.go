package mcp

import (
	"context"
	"slices"
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

		// Expected: 1 string flag (2 args) + 2 bool flags + "--" + name = 6 args
		if len(args) != len(stringFlags)*2+len(boolFlags)+2 {
			t.Fatalf("expected %d args, got %d: %v", len(stringFlags)*2+len(boolFlags)+2, len(args), args)
		}

		// Validate the positional argument (name) comes last, after "--"
		if args[len(args)-2] != "--" || args[len(args)-1] != name {
			t.Fatalf("expected args to end with [-- %s], got %v", name, args)
		}

		// Validate flags (excluding "--" and the name at the end)
		validateStringFlags(t, args[:len(args)-2], stringFlags)
		validateBoolFlags(t, args[:len(args)-2], boolFlags)

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

// TestTool_Delete_InvalidName ensures the delete tool rejects a name that is
// not a valid Function name, before anything is executed. Without this, a
// name such as "--all" would be parsed by the CLI as a flag, and the CLI
// would fall back to deleting the Function in the server's working directory.
func TestTool_Delete_InvalidName(t *testing.T) {
	for _, name := range []string{"--all", "-h", "--namespace=kube-system", "My_Function", ""} {
		t.Run(name, func(t *testing.T) {
			executor := mock.NewExecutor()
			client, server, err := newTestPair(t, WithExecutor(executor))
			if err != nil {
				t.Fatal(err)
			}
			server.readonly.Store(false)

			result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "delete",
				Arguments: map[string]any{"name": name},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("expected error result for name %q, got success", name)
			}
			if executor.ExecuteInvoked {
				t.Fatalf("executor was invoked for invalid name %q", name)
			}
		})
	}
}

// TestDeleteInput_Args_NameAfterDoubleDash ensures the name is always passed
// after "--", so the CLI can never parse it as a flag, even if handler
// validation were bypassed.
func TestDeleteInput_Args_NameAfterDoubleDash(t *testing.T) {
	args := DeleteInput{Name: ptr("--all"), Namespace: ptr("prod")}.Args()
	want := []string{"--namespace", "prod", "--", "--all"}
	if !slices.Equal(args, want) {
		t.Fatalf("expected %v, got %v", want, args)
	}
}
