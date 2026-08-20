package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Version verifies the version tool executes "version --output json"
// and maps the result into VersionOutput.
func TestTool_Version(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "version" {
			t.Fatalf("expected subcommand 'version', got %q", subcommand)
		}
		validateArgLength(t, args, 1, 0)
		validateStringFlags(t, args, map[string]struct {
			jsonKey string
			flag    string
			value   string
		}{
			"output": {"output", "--output", "json"},
		})
		return []byte(`{"version":"v1.16.0","commit":"abc123"}`), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "version",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("version tool call failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("version returned an error result: %v", resultToString(result))
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}

	var output VersionOutput
	if err := json.Unmarshal([]byte(resultToString(result)), &output); err != nil {
		t.Fatalf("failed to parse version output as JSON: %v", err)
	}
	if output.Version != "v1.16.0" {
		t.Errorf("expected version 'v1.16.0', got %q", output.Version)
	}
	if output.GitRevision != "abc123" {
		t.Errorf("expected gitRevision 'abc123', got %q", output.GitRevision)
	}
}

// TestTool_Version_NoCommit verifies a missing "commit" field (e.g. a build
// from source without ldflags) results in an empty GitRevision, not an error.
func TestTool_Version_NoCommit(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte(`{"version":"v0.0.0+source"}`), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "version",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("version tool call failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("version returned an error result: %v", resultToString(result))
	}

	var output VersionOutput
	if err := json.Unmarshal([]byte(resultToString(result)), &output); err != nil {
		t.Fatalf("failed to parse version output as JSON: %v", err)
	}
	if output.Version != "v0.0.0+source" {
		t.Errorf("expected version 'v0.0.0+source', got %q", output.Version)
	}
	if output.GitRevision != "" {
		t.Errorf("expected empty gitRevision, got %q", output.GitRevision)
	}
}

// TestTool_Version_ExecutorError verifies an executor failure surfaces as an
// error result rather than a panic or malformed output.
func TestTool_Version_ExecutorError(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "version",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected transport-level error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when the executor fails")
	}
}

// TestTool_Version_MalformedJSON verifies malformed JSON from the executor
// surfaces as an error result rather than a panic.
func TestTool_Version_MalformedJSON(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "version",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected transport-level error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when the executor returns malformed JSON")
	}
}
