package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigLabelsAdd ensures the config_labels_add tool executes with all arguments.
func TestTool_ConfigLabelsAdd(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":  {"path", "--path", "."},
		"name":  {"name", "--name", "environment"},
		"value": {"value", "--value", "prod"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
		}

		if args[0] != "labels" {
			t.Fatalf("expected args[0]='labels', got %q", args[0])
		}
		if args[1] != "add" {
			t.Fatalf("expected args[1]='add', got %q", args[1])
		}

		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Label added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_add",
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

// TestTool_ConfigLabelsList ensures the config_labels_list tool lists labels.
func TestTool_ConfigLabelsList(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// "labels" + "--path" + "." = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "labels" {
			t.Fatalf("expected args[0]='labels', got %q", args[0])
		}

		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path='.', got %q", val)
		}

		return []byte("app=my-function\nenvironment=prod\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_list",
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

// TestTool_ConfigLabelsRemove ensures the config_labels_remove tool removes a label.
func TestTool_ConfigLabelsRemove(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path": {"path", "--path", "."},
		"name": {"name", "--name", "environment"},
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
		}

		if args[0] != "labels" {
			t.Fatalf("expected args[0]='labels', got %q", args[0])
		}
		if args[1] != "remove" {
			t.Fatalf("expected args[1]='remove', got %q", args[1])
		}

		validateArgLength(t, args[2:], len(stringFlags), 0)
		validateStringFlags(t, args[2:], stringFlags)

		return []byte("Label removed successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, nil)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_remove",
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

// TestTool_ConfigLabelsList_Error ensures the config_labels_list tool propagates executor errors.
func TestTool_ConfigLabelsList_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("list failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_list",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigLabelsAdd_Error ensures the config_labels_add tool propagates executor errors.
func TestTool_ConfigLabelsAdd_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("add failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_add",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigLabelsRemove_Error ensures the config_labels_remove tool propagates executor errors.
func TestTool_ConfigLabelsRemove_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("remove failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_remove",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigLabelsAdd_Readonly ensures the config_labels_add tool is blocked in readonly mode.
func TestTool_ConfigLabelsAdd_Readonly(t *testing.T) {
	executor := mock.NewExecutor()

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(true)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_add",
		Arguments: map[string]any{"path": ".", "name": "app", "value": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result in readonly mode, got success")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not be invoked in readonly mode")
	}
}

// TestTool_ConfigLabelsRemove_Readonly ensures the config_labels_remove tool is blocked in readonly mode.
func TestTool_ConfigLabelsRemove_Readonly(t *testing.T) {
	executor := mock.NewExecutor()

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(true)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_labels_remove",
		Arguments: map[string]any{"path": ".", "name": "app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result in readonly mode, got success")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not be invoked in readonly mode")
	}
}
