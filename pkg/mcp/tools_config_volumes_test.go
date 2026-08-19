package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_ConfigVolumesAdd(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_volumes_add",
		Arguments: map[string]any{
			"path":      initTestFunction(t),
			"type":      "emptydir",
			"mountPath": "/tmp/cache",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

func TestTool_ConfigVolumesList(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_volumes_list",
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

func TestTool_ConfigVolumesRemove(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)
	volType := "emptydir"
	mountPath := "/tmp/cache"
	if _, err := svc.ConfigVolumesAdd(context.Background(), ConfigVolumesAddInput{
		Path: path, Type: &volType, MountPath: &mountPath,
	}); err != nil {
		t.Fatal(err)
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_volumes_remove",
		Arguments: map[string]any{
			"path":      path,
			"mountPath": mountPath,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}
