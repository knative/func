package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_RepositoryList ensures the repository_list tool invokes the executor correctly.
func TestTool_RepositoryList(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "repository" {
			t.Fatalf("expected subcommand 'repository', got %q", subcommand)
		}
		if len(args) < 1 || args[0] != "list" {
			t.Fatalf("expected args[0]='list', got %v", args)
		}
		return []byte("default\nboson\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_list",
		Arguments: map[string]any{},
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

// TestTool_RepositoryList_Verbose ensures --verbose is passed when requested.
func TestTool_RepositoryList_Verbose(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		argsMap := argsToMap(args)
		if _, ok := argsMap["--verbose"]; !ok {
			t.Fatalf("expected --verbose flag, got args: %v", args)
		}
		return []byte("default\nboson\thttps://github.com/boson-project/templates\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_list",
		Arguments: map[string]any{"verbose": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

// TestTool_RepositoryList_Error ensures executor errors are propagated.
func TestTool_RepositoryList_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("list failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_RepositoryAdd ensures the repository_add tool passes name and URL correctly.
func TestTool_RepositoryAdd(t *testing.T) {
	const (
		repoName = "boson"
		repoURL  = "https://github.com/boson-project/templates"
	)

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "repository" {
			t.Fatalf("expected subcommand 'repository', got %q", subcommand)
		}
		// args: ["add", <name>, <url>]
		if len(args) < 3 {
			t.Fatalf("expected at least 3 args (add <name> <url>), got %d: %v", len(args), args)
		}
		if args[0] != "add" {
			t.Fatalf("expected args[0]='add', got %q", args[0])
		}
		if args[1] != repoName {
			t.Fatalf("expected args[1]=%q, got %q", repoName, args[1])
		}
		if args[2] != repoURL {
			t.Fatalf("expected args[2]=%q, got %q", repoURL, args[2])
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_add",
		Arguments: map[string]any{"name": repoName, "url": repoURL},
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

// TestTool_RepositoryAdd_Readonly ensures repository_add is blocked in readonly mode.
func TestTool_RepositoryAdd_Readonly(t *testing.T) {
	executor := mock.NewExecutor()
	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(true)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_add",
		Arguments: map[string]any{"name": "boson", "url": "https://github.com/boson-project/templates"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result in readonly mode, got success")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not have been invoked in readonly mode")
	}
}

// TestTool_RepositoryAdd_Error ensures executor errors are propagated.
func TestTool_RepositoryAdd_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("add failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_add",
		Arguments: map[string]any{"name": "boson", "url": "https://github.com/boson-project/templates"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_RepositoryRename ensures the repository_rename tool passes old and new names correctly.
func TestTool_RepositoryRename(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "repository" {
			t.Fatalf("expected subcommand 'repository', got %q", subcommand)
		}
		// args: ["rename", <old>, <new>]
		if len(args) < 3 {
			t.Fatalf("expected at least 3 args (rename <old> <new>), got %d: %v", len(args), args)
		}
		if args[0] != "rename" {
			t.Fatalf("expected args[0]='rename', got %q", args[0])
		}
		if args[1] != "boson" {
			t.Fatalf("expected args[1]='boson', got %q", args[1])
		}
		if args[2] != "functastic" {
			t.Fatalf("expected args[2]='functastic', got %q", args[2])
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_rename",
		Arguments: map[string]any{"old": "boson", "new": "functastic"},
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

// TestTool_RepositoryRename_Readonly ensures repository_rename is blocked in readonly mode.
func TestTool_RepositoryRename_Readonly(t *testing.T) {
	executor := mock.NewExecutor()
	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(true)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_rename",
		Arguments: map[string]any{"old": "boson", "new": "functastic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result in readonly mode, got success")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not have been invoked in readonly mode")
	}
}

// TestTool_RepositoryRename_Error ensures executor errors are propagated.
func TestTool_RepositoryRename_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("rename failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_rename",
		Arguments: map[string]any{"old": "boson", "new": "functastic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_RepositoryRemove ensures the repository_remove tool passes the name correctly.
func TestTool_RepositoryRemove(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "repository" {
			t.Fatalf("expected subcommand 'repository', got %q", subcommand)
		}
		// args: ["remove", <name>]
		if len(args) < 2 {
			t.Fatalf("expected at least 2 args (remove <name>), got %d: %v", len(args), args)
		}
		if args[0] != "remove" {
			t.Fatalf("expected args[0]='remove', got %q", args[0])
		}
		if args[1] != "boson" {
			t.Fatalf("expected args[1]='boson', got %q", args[1])
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_remove",
		Arguments: map[string]any{"name": "boson"},
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

// TestTool_RepositoryRemove_Readonly ensures repository_remove is blocked in readonly mode.
func TestTool_RepositoryRemove_Readonly(t *testing.T) {
	executor := mock.NewExecutor()
	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(true)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_remove",
		Arguments: map[string]any{"name": "boson"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result in readonly mode, got success")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor should not have been invoked in readonly mode")
	}
}

// TestTool_RepositoryRemove_Error ensures executor errors are propagated.
func TestTool_RepositoryRemove_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("remove failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "repository_remove",
		Arguments: map[string]any{"name": "boson"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}
