package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestTool_Healthcheck_ServerAlive verifies the server-liveness fields are
// always present and correct regardless of cluster state.
func TestTool_Healthcheck_ServerAlive(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "healthcheck",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("healthcheck tool call failed: %v", err)
	}
	if result.IsError {
		t.Fatal("healthcheck returned an error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("healthcheck returned no content")
	}

	output := mustParseHealthcheckOutput(t, result)

	if output.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", output.Status)
	}
	if output.Message == "" {
		t.Error("expected non-empty message")
	}
	if !strings.Contains(output.Message, "running") {
		t.Errorf("expected message to contain 'running', got %q", output.Message)
	}
	if output.Version == "" {
		t.Error("expected non-empty version")
	}
}

// TestTool_Healthcheck_ClusterReachable verifies that when the functions
// client can list functions successfully, ClusterConnected is true.
//
// Migration note (#3771): uses mock.NewLister() from pkg/mock — no subprocess.
func TestTool_Healthcheck_ClusterReachable(t *testing.T) {
	// mock.NewLister() returns an empty list with no error by default,
	// simulating a reachable but empty cluster.
	lister := mock.NewLister()
	funcClient := fn.New(fn.WithListers(lister))

	client, _, err := newTestPair(t, WithClient(funcClient))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "healthcheck",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("healthcheck tool call failed: %v", err)
	}

	output := mustParseHealthcheckOutput(t, result)

	if !output.ClusterConnected {
		t.Errorf("expected clusterConnected=true, got false (clusterMessage: %s)", output.ClusterMessage)
	}
	if output.ClusterMessage == "" {
		t.Error("expected non-empty clusterMessage when connected")
	}
	if !lister.ListInvoked {
		t.Error("expected lister.List to be invoked")
	}
}

// TestTool_Healthcheck_ClusterUnreachable verifies that when the functions
// client cannot reach the cluster, ClusterConnected is false and
// ClusterMessage explains why — without IsError or a Go-level error.
func TestTool_Healthcheck_ClusterUnreachable(t *testing.T) {
	// errLister always returns an error, simulating a broken cluster.
	lister := &errLister{err: errors.New("connection refused: no kubeconfig found")}
	funcClient := fn.New(fn.WithListers(lister))

	client, _, err := newTestPair(t, WithClient(funcClient))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "healthcheck",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("healthcheck must not return a Go-level error: %v", err)
	}
	// Cluster failure must NOT mark the tool result as an error — the server
	// is still alive; the agent needs to decide what to do with the info.
	if result.IsError {
		t.Fatal("healthcheck must not set IsError=true for cluster failures")
	}

	output := mustParseHealthcheckOutput(t, result)

	if output.Status != "ok" {
		t.Errorf("expected status 'ok' even when cluster is down, got %q", output.Status)
	}
	if output.ClusterConnected {
		t.Error("expected clusterConnected=false when lister returns an error")
	}
	if output.ClusterMessage == "" {
		t.Error("expected non-empty clusterMessage explaining the failure")
	}
}

// TestTool_Healthcheck_StructuredOutputShape verifies all JSON keys are
// present so agents can rely on a stable schema.
func TestTool_Healthcheck_StructuredOutputShape(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "healthcheck",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	for _, key := range []string{"status", "message", "version", "clusterConnected", "clusterMessage"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("output JSON is missing required key %q", key)
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// errLister is a Lister that always returns the configured error.
// It is used in tests to simulate an unreachable Kubernetes cluster without
// any subprocess or real network call.
type errLister struct {
	err error
}

func (e *errLister) List(_ context.Context, _ string) ([]fn.ListItem, error) {
	return nil, e.err
}

// mustParseHealthcheckOutput extracts and unmarshals the JSON payload from a
// CallToolResult, failing the test immediately on any problem.
func mustParseHealthcheckOutput(t *testing.T, result *mcp.CallToolResult) HealthcheckOutput {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	var output HealthcheckOutput
	if err := json.Unmarshal([]byte(textContent.Text), &output); err != nil {
		t.Fatalf("failed to parse HealthcheckOutput: %v\nraw: %s", err, textContent.Text)
	}
	return output
}
