package mcp

import (
	"context"
	"errors"
	"slices"
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
		want := []string{"add", "--", repoName, repoURL}
		if !slices.Equal(args, want) {
			t.Fatalf("expected args %v, got %v", want, args)
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
		want := []string{"rename", "--", "boson", "functastic"}
		if !slices.Equal(args, want) {
			t.Fatalf("expected args %v, got %v", want, args)
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
		want := []string{"remove", "--", "boson"}
		if !slices.Equal(args, want) {
			t.Fatalf("expected args %v, got %v", want, args)
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

// TestRepositoryInputs_Args_PositionalsAfterDoubleDash ensures repository
// names and URLs are always passed after "--", after any flags, so a value
// starting with "-" (e.g. "--help") is never parsed by the CLI as a flag.
func TestRepositoryInputs_Args_PositionalsAfterDoubleDash(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "add",
			args: RepositoryAddInput{Name: "--help", URL: "--verbose", Verbose: ptr(true)}.Args(),
			want: []string{"add", "--verbose", "--", "--help", "--verbose"},
		},
		{
			name: "rename",
			args: RepositoryRenameInput{Old: "--help", New: "-v"}.Args(),
			want: []string{"rename", "--", "--help", "-v"},
		},
		{
			name: "remove",
			args: RepositoryRemoveInput{Name: "--help"}.Args(),
			want: []string{"remove", "--", "--help"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Equal(tt.args, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, tt.args)
			}
		})
	}
}
