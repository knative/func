package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Build_ReadonlyPushRejected ensures build with push=true is rejected in readonly mode.
func TestTool_Build_ReadonlyPushRejected(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true) // readonly = true
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "build",
		Arguments: map[string]any{"path": ".", "push": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected build --push to be rejected in readonly mode")
	}
}

// TestTool_Build_ReadonlyWithoutPushAllowed ensures build without push is allowed in readonly mode.
func TestTool_Build_ReadonlyWithoutPushAllowed(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(_ context.Context, subcommand string, _ ...string) ([]byte, error) {
		if subcommand != "build" {
			t.Fatalf("expected subcommand 'build', got %q", subcommand)
		}
		return []byte("OK\n"), nil
	}

	client, _, err := newTestPair(t, WithReadonly(true), WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "build",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected build without push to succeed in readonly mode, got error: %v", result)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_Build_Args ensures the build tool executes with all arguments passed correctly.
func TestTool_Build_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":         {"path", "--path", "."},
		"builder":      {"builder", "--builder", "pack"},
		"registry":     {"registry", "--registry", "ghcr.io/user"},
		"builderImage": {"builderImage", "--builder-image", "custom-builder:latest"},
		"image":        {"image", "--image", "ghcr.io/user/my-func:latest"},
		"platform":     {"platform", "--platform", "linux/amd64"},
	}

	boolFlags := map[string]string{
		"push":             "--push",
		"registryInsecure": "--registry-insecure",
		"buildTimestamp":   "--build-timestamp",
		"verbose":          "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "build" {
			t.Fatalf("expected subcommand 'build', got %q", subcommand)
		}

		validateArgLength(t, args, len(stringFlags), len(boolFlags))
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

	// Invoke tool with all optional arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "build",
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
