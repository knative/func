package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_ConfigEnvsAdd(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	path := initTestFunction(t)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":  path,
			"name":  "API_KEY",
			"value": "secret123",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

func TestTool_ConfigEnvsList(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_list",
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

func TestTool_ConfigEnvsRemove(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)
	name := "TO_REMOVE"
	val := "x"
	if _, err := svc.ConfigEnvsAdd(context.Background(), ConfigEnvsAddInput{Path: path, Name: &name, Value: &val}); err != nil {
		t.Fatal(err)
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_remove",
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
