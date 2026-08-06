package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Delete_ReadonlyRejected(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true)
	if err != nil {
		t.Fatal(err)
	}

	name := "myfunc"
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "delete",
		Arguments: map[string]any{
			"name": name,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected readonly mode to reject delete")
	}
}
