package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigGitSet_Args ensures the config_git_set tool passes all
// arguments correctly to the executor.
func TestTool_ConfigGitSet_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":       {"path", "--path", "/home/user/myfunc"},
		"git_url":    {"git_url", "--git-url", "https://github.com/user/repo"},
		"git_branch": {"git_branch", "--git-branch", "main"},
		"git_dir":    {"git_dir", "--git-dir", "functions/myfunc"},
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
		if args[0] != "git" {
			t.Fatalf("expected args[0]='git', got %q", args[0])
		}
		if args[1] != "set" {
			t.Fatalf("expected args[1]='set', got %q", args[1])
		}

		// After "git set": --path, path, --git-url, url, --git-branch, branch, --git-dir, dir = 8 args
		// + --config-local (1) + --verbose (1) = 10 args after "git set"
		remaining := args[2:]
		validateStringFlags(t, remaining, stringFlags)
		validateBoolFlags(t, remaining, boolFlags)

		return []byte("Git configuration set successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_git_set",
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

// TestTool_ConfigGitSet_DefaultGitDir ensures that when git_dir is not
// provided, "--git-dir ." is forwarded to prevent the interactive prompt.
func TestTool_ConfigGitSet_DefaultGitDir(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		argsMap := argsToMap(args)
		val, ok := argsMap["--git-dir"]
		if !ok {
			t.Fatal("expected --git-dir flag to be present, got none")
		}
		if val != "." {
			t.Fatalf("expected --git-dir='.', got %q", val)
		}
		return []byte("ok\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_git_set",
		Arguments: map[string]any{
			"path":       "/home/user/myfunc",
			"git_url":    "https://github.com/user/repo",
			"git_branch": "main",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

// TestTool_ConfigGitSet_AntiHang_ConfigLocal ensures that when no config-*
// flags are provided, --config-local is automatically forwarded to prevent
// the interactive webhook confirmation prompt in a non-TTY subprocess.
func TestTool_ConfigGitSet_AntiHang_ConfigLocal(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		argsMap := argsToMap(args)
		if _, ok := argsMap["--config-local"]; !ok {
			t.Fatal("expected --config-local to be present when no config-* flags are provided")
		}
		return []byte("ok\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_git_set",
		Arguments: map[string]any{
			"path":       "/home/user/myfunc",
			"git_url":    "https://github.com/user/repo",
			"git_branch": "main",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

// TestTool_ConfigGitSet_MissingGitURL ensures that omitting git_url returns
// an error without invoking the executor.
func TestTool_ConfigGitSet_MissingGitURL(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor should not be called when git_url is missing")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_git_set",
		Arguments: map[string]any{
			"path":       "/home/user/myfunc",
			"git_branch": "main",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when git_url is missing")
	}
}

// TestTool_ConfigGitSet_MissingGitBranch ensures that omitting git_branch
// returns an error without invoking the executor.
func TestTool_ConfigGitSet_MissingGitBranch(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor should not be called when git_branch is missing")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_git_set",
		Arguments: map[string]any{
			"path":    "/home/user/myfunc",
			"git_url": "https://github.com/user/repo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when git_branch is missing")
	}
}

// TestTool_ConfigGitSet_Error ensures executor errors are propagated.
func TestTool_ConfigGitSet_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("set failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_git_set",
		Arguments: map[string]any{
			"path":       "/home/user/myfunc",
			"git_url":    "https://github.com/user/repo",
			"git_branch": "main",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigGitRemove_Args ensures the config_git_remove tool passes
// all arguments correctly to the executor.
func TestTool_ConfigGitRemove_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path": {"path", "--path", "/home/user/myfunc"},
	}

	boolFlags := map[string]string{
		"delete_local":   "--delete-local",
		"delete_cluster": "--delete-cluster",
		"verbose":        "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 {
			t.Fatalf("expected at least 2 args ('git', 'remove'), got %d: %v", len(args), args)
		}
		if args[0] != "git" {
			t.Fatalf("expected args[0]='git', got %q", args[0])
		}
		if args[1] != "remove" {
			t.Fatalf("expected args[1]='remove', got %q", args[1])
		}

		remaining := args[2:]
		validateStringFlags(t, remaining, stringFlags)
		validateBoolFlags(t, remaining, boolFlags)

		return []byte("Git configuration removed\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_git_remove",
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

// TestTool_ConfigGitRemove_AntiHang_DeleteLocal ensures that when neither
// delete-local nor delete-cluster is provided, --delete-local is forwarded
// automatically to prevent the interactive "delete all?" prompt.
func TestTool_ConfigGitRemove_AntiHang_DeleteLocal(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		argsMap := argsToMap(args)
		if _, ok := argsMap["--delete-local"]; !ok {
			t.Fatal("expected --delete-local to be present when no delete-* flags are provided")
		}
		return []byte("ok\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_git_remove",
		Arguments: map[string]any{"path": "/home/user/myfunc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
}

// TestTool_ConfigGitRemove_Error ensures executor errors are propagated.
func TestTool_ConfigGitRemove_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("remove failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_git_remove",
		Arguments: map[string]any{"path": "/home/user/myfunc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}
