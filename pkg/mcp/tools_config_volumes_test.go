package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigVolumesAdd ensures the config_volumes_add tool executes with all arguments.
func TestTool_ConfigVolumesAdd(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":      {"path", "--path", "."},
		"type":      {"type", "--type", "secret"},
		"mountPath": {"mountPath", "--mount-path", "/workspace/secret"},
		"source":    {"source", "--source", "my-secret"},
		"medium":    {"medium", "--medium", "Memory"},
		"size":      {"size", "--size", "1Gi"},
	}

	boolFlags := map[string]string{
		"readOnly": "--read-only",
		"verbose":  "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
		}

		if args[0] != "volumes" {
			t.Fatalf("expected args[0]='volumes', got %q", args[0])
		}
		if args[1] != "add" {
			t.Fatalf("expected args[1]='add', got %q", args[1])
		}

		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Volume added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_add",
		Arguments: inputArgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_ConfigVolumesList ensures the config_volumes_list tool lists volumes.
func TestTool_ConfigVolumesList(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// "volumes" + "--path" + "." = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "volumes" {
			t.Fatalf("expected args[0]='volumes', got %q", args[0])
		}

		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path='.', got %q", val)
		}

		return []byte("secret:my-secret:/workspace/secret\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_list",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_ConfigVolumesRemove ensures the config_volumes_remove tool removes a volume.
func TestTool_ConfigVolumesRemove(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":      {"path", "--path", "."},
		"mountPath": {"mountPath", "--mount-path", "/workspace/secret"},
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
		}

		if args[0] != "volumes" {
			t.Fatalf("expected args[0]='volumes', got %q", args[0])
		}
		if args[1] != "remove" {
			t.Fatalf("expected args[1]='remove', got %q", args[1])
		}

		validateArgLength(t, args[2:], len(stringFlags), 0)
		validateStringFlags(t, args[2:], stringFlags)

		return []byte("Volume removed successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, nil)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_remove",
		Arguments: inputArgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

// TestTool_ConfigVolumesRemove_EmptyMountPath ensures the config_volumes_remove tool rejects an empty mountPath.
func TestTool_ConfigVolumesRemove_EmptyMountPath(t *testing.T) {
	executor := mock.NewExecutor()
	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_remove",
		Arguments: map[string]any{"path": ".", "mountPath": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when mountPath is empty")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when mountPath is empty")
	}
}

// TestTool_ConfigVolumesRemove_MissingMountPath ensures the config_volumes_remove tool rejects calls without mountPath.
func TestTool_ConfigVolumesRemove_MissingMountPath(t *testing.T) {
	executor := mock.NewExecutor()
	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_remove",
		Arguments: map[string]any{"path": "."},
	})
	// Schema validation may produce a protocol-level error or a tool-level error result.
	if err == nil && !result.IsError {
		t.Fatal("expected error when mountPath is missing")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when required fields are absent")
	}
}

// TestTool_ConfigVolumesList_Error ensures the config_volumes_list tool propagates executor errors.
func TestTool_ConfigVolumesList_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("list failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_list",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigVolumesAdd_Error ensures the config_volumes_add tool propagates executor errors.
func TestTool_ConfigVolumesAdd_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("add failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_add",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigVolumesRemove_Error ensures the config_volumes_remove tool propagates executor errors.
func TestTool_ConfigVolumesRemove_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("remove failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_volumes_remove",
		Arguments: map[string]any{"path": ".", "mountPath": "/workspace/secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}
