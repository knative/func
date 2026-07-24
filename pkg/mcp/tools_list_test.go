package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_List(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}
