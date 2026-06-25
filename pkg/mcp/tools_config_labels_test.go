package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_ConfigLabelsAdd(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_labels_add",
		Arguments: map[string]any{
			"path":  initTestFunction(t),
			"name":  "app",
			"value": "demo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

func TestTool_ConfigLabelsList(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_labels_list",
		Arguments: map[string]any{
			"path": initTestFunction(t),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

func TestTool_ConfigLabelsRemove(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)
	name := "app"
	value := "demo"
	if _, err := svc.ConfigLabelsAdd(context.Background(), ConfigLabelsAddInput{Path: path, Name: &name, Value: &value}); err != nil {
		t.Fatal(err)
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_labels_remove",
		Arguments: map[string]any{
			"path": path,
			"name": name,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}
