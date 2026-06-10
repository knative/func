package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Create(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "create",
		Arguments: map[string]any{
			"language": "go",
			"path":     dir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}
