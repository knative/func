package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigCI ensures the config_ci tool executes with all arguments forwarded.
func TestTool_ConfigCI(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":                         {"path", "--path", "."},
		"branch":                       {"branch", "--branch", "main"},
		"workflowName":                 {"workflowName", "--workflow-name", "Func Deploy"},
		"kubeconfigSecretName":         {"kubeconfigSecretName", "--kubeconfig-secret-name", "KUBECONFIG"},
		"registryLoginUrlVariableName": {"registryLoginUrlVariableName", "--registry-login-url-variable-name", "REGISTRY_LOGIN_URL"},
		"registryUserVariableName":     {"registryUserVariableName", "--registry-user-variable-name", "REGISTRY_USERNAME"},
		"registryPassSecretName":       {"registryPassSecretName", "--registry-pass-secret-name", "REGISTRY_PASSWORD"},
		"registryUrlVariableName":      {"registryUrlVariableName", "--registry-url-variable-name", "REGISTRY_URL"},
	}

	boolFlags := map[string]string{
		"registryLogin":    "--registry-login",
		"remote":           "--remote",
		"selfHostedRunner": "--self-hosted-runner",
		"testStep":         "--test-step",
		"force":            "--force",
		"verbose":          "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 1 {
			t.Fatalf("expected at least 1 arg, got %d: %v", len(args), args)
		}

		if args[0] != "ci" {
			t.Fatalf("expected args[0]='ci', got %q", args[0])
		}

		// args[1:] are the flags
		validateArgLength(t, args[1:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[1:], stringFlags)
		validateBoolFlags(t, args[1:], boolFlags)

		return []byte("GitHub workflow generated successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_ci",
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

// TestTool_ConfigCI_MinimalArgs ensures the config_ci tool executes with only the
// required 'path' parameter, omitting all optional flags.
func TestTool_ConfigCI_MinimalArgs(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// "ci" + "--path" + "." = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "ci" {
			t.Fatalf("expected args[0]='ci', got %q", args[0])
		}

		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path='.', got %q", val)
		}

		return []byte("GitHub workflow generated successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_ci",
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

// TestTool_ConfigCI_Error ensures the config_ci tool propagates executor errors,
// such as the "unknown command" error returned when FUNC_ENABLE_CI_CONFIG is not set.
func TestTool_ConfigCI_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte(`unknown command "ci" for "config"`), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_ci",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}
