package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestTool_Healthcheck verifies the healthcheck tool returns expected output
func TestTool_Healthcheck(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	// Invoke the healthcheck tool
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "healthcheck",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("healthcheck tool call failed: %v", err)
	}

	// Verify the result is not an error
	if result.IsError {
		t.Fatal("healthcheck returned an error result")
	}

	// Verify we got content back
	if len(result.Content) == 0 {
		t.Fatal("healthcheck returned no content")
	}

	// Verify the content contains expected fields
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent in healthcheck result")
	}

	// The response should be JSON with status and message
	if textContent.Text == "" {
		t.Fatal("healthcheck returned empty text content")
	}

	// Parse the JSON output to verify structure
	var output HealthcheckOutput
	if err := json.Unmarshal([]byte(textContent.Text), &output); err != nil {
		t.Fatalf("failed to parse healthcheck output as JSON: %v", err)
	}

	// Verify expected fields
	if output.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", output.Status)
	}

	if output.Message == "" {
		t.Error("expected non-empty message")
	}

	if !strings.Contains(output.Message, "running") {
		t.Errorf("expected message to contain 'running', got %q", output.Message)
	}
}
