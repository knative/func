package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_RunStop_Readonly ensures the run_stop tool rejects requests in
// readonly mode.
func TestTool_RunStop_Readonly(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_stop",
		Arguments: map[string]any{"path": "/tmp/my-func"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected run_stop to be rejected in readonly mode")
	}
}

// TestTool_RunStop_NotRunning ensures a clear error is returned when there
// is no active run for the given path.
func TestTool_RunStop_NotRunning(t *testing.T) {
	client, server, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_stop",
		Arguments: map[string]any{"path": "/tmp/never-ran"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected run_stop to fail for a path with no active run")
	}
}

// TestTool_RunStop_Success ensures run_stop invokes the stop function
// returned by a prior run, removes the registry entry, and that a
// subsequent run_stop for the same path then fails clearly.
func TestTool_RunStop_Success(t *testing.T) {
	stopped := false
	starter := mock.NewProcessStarter()
	starter.StartFn = func(ctx context.Context, subcommand string, args ...string) (int, string, string, func() error, error) {
		return 555, "127.0.0.1", "8080", func() error { stopped = true; return nil }, nil
	}

	client, server, err := newTestPair(t, WithProcessStarter(starter))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	path := "/tmp/stop-me"

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run",
		Arguments: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error starting run: %v", result)
	}

	result, err = client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_stop",
		Arguments: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error from run_stop: %v", result)
	}
	if !stopped {
		t.Fatal("expected the stop function to have been invoked")
	}

	// A second run_stop for the same, now-inactive path must fail clearly.
	result, err = client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_stop",
		Arguments: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected second run_stop for the same path to fail")
	}
}
