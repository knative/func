package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

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

// TestTool_Build_StructuredOutput verifies that the Image field is populated
// from the .func/built-image file written by the func CLI after a successful build.
func TestTool_Build_StructuredOutput(t *testing.T) {
	const wantImage = "ghcr.io/user/my-func:latest"

	// Create a minimal function directory that fn.NewFunction can read.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "func.yaml"), []byte("name: my-func\nruntime: go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	funcDir := filepath.Join(root, ".func")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "built-image"), []byte(wantImage), 0644); err != nil {
		t.Fatal(err)
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("Build successful\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "build",
		Arguments: map[string]any{"path": root},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	raw := resultToString(result)
	var output BuildOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("failed to unmarshal output: %v\nraw: %s", err, raw)
	}
	if output.Image != wantImage {
		t.Errorf("Image = %q, want %q", output.Image, wantImage)
	}
}

// TestTool_Build_StructuredOutput_NoFuncYaml verifies that a missing func.yaml
// (e.g. an invalid path) does not cause the handler to fail — Image is just empty.
func TestTool_Build_StructuredOutput_NoFuncYaml(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("Build successful\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "build",
		Arguments: map[string]any{"path": t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	raw := resultToString(result)
	var output BuildOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("failed to unmarshal output: %v\nraw: %s", err, raw)
	}
	if output.Image != "" {
		t.Errorf("expected empty Image when func.yaml absent, got %q", output.Image)
	}
}
