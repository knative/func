package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Build_RequiresFunction(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "build",
		Arguments: map[string]any{
			"path": initTestFunction(t),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Build may fail without docker/buildpacks; verify handler routed to service (not missing factory).
	if result.IsError {
		text, _ := result.Content[0].(*mcp.TextContent)
		if text != nil && text.Text == "mcp server not configured with a client factory" {
			t.Fatalf("unexpected configuration error: %s", text.Text)
		}
	}
}
