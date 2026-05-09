package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_ConfigEnvsAdd ensures the config_envs_add tool executes with all arguments.
func TestTool_ConfigEnvsAdd(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":  {"path", "--path", "."},
		"name":  {"name", "--name", "API_KEY"},
		"value": {"value", "--value", "secret123"},
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

		if args[0] != "envs" {
			t.Fatalf("expected args[0]='envs', got %q", args[0])
		}
		if args[1] != "add" {
			t.Fatalf("expected args[1]='add', got %q", args[1])
		}

		// args[2:] are the flags
		validateArgLength(t, args[2:], len(stringFlags), len(boolFlags))
		validateStringFlags(t, args[2:], stringFlags)
		validateBoolFlags(t, args[2:], boolFlags)

		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_add",
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

// TestTool_ConfigEnvsAdd_SecretKey ensures secret-key-sourced env vars produce
// the correct "{{ secret:name:key }}" value template.
func TestTool_ConfigEnvsAdd_SecretKey(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 || args[0] != "envs" || args[1] != "add" {
			t.Fatalf("expected positional args ['envs','add'], got %v", args[:min(2, len(args))])
		}
		argsMap := argsToMap(args[2:])
		wantValue := "{{ secret:my-secret:MY_KEY }}"
		if got, ok := argsMap["--value"]; !ok || got != wantValue {
			t.Fatalf("expected --value=%q, got %q", wantValue, got)
		}
		if got, ok := argsMap["--name"]; !ok || got != "API_KEY" {
			t.Fatalf("expected --name='API_KEY', got %q", got)
		}
		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"name":       "API_KEY",
			"secretName": "my-secret",
			"secretKey":  "MY_KEY",
		},
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

// TestTool_ConfigEnvsAdd_SecretAllKeys ensures importing all keys from a Secret
// produces the "{{ secret:name }}" value template without --name.
func TestTool_ConfigEnvsAdd_SecretAllKeys(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 || args[0] != "envs" || args[1] != "add" {
			t.Fatalf("expected positional args ['envs','add'], got %v", args[:min(2, len(args))])
		}
		argsMap := argsToMap(args[2:])
		wantValue := "{{ secret:my-secret }}"
		if got, ok := argsMap["--value"]; !ok || got != wantValue {
			t.Fatalf("expected --value=%q, got %q", wantValue, got)
		}
		if _, ok := argsMap["--name"]; ok {
			t.Fatal("expected no --name flag when importing all secret keys")
		}
		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"secretName": "my-secret",
		},
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

// TestTool_ConfigEnvsAdd_ConfigMapKey ensures configmap-key-sourced env vars produce
// the correct "{{ configMap:name:key }}" value template.
func TestTool_ConfigEnvsAdd_ConfigMapKey(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 || args[0] != "envs" || args[1] != "add" {
			t.Fatalf("expected positional args ['envs','add'], got %v", args[:min(2, len(args))])
		}
		argsMap := argsToMap(args[2:])
		wantValue := "{{ configMap:my-config:DB_HOST }}"
		if got, ok := argsMap["--value"]; !ok || got != wantValue {
			t.Fatalf("expected --value=%q, got %q", wantValue, got)
		}
		if got, ok := argsMap["--name"]; !ok || got != "DATABASE_HOST" {
			t.Fatalf("expected --name='DATABASE_HOST', got %q", got)
		}
		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":          ".",
			"name":          "DATABASE_HOST",
			"configMapName": "my-config",
			"configMapKey":  "DB_HOST",
		},
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

// TestTool_ConfigEnvsAdd_ConfigMapAllKeys ensures importing all keys from a ConfigMap
// produces the "{{ configMap:name }}" value template without --name.
func TestTool_ConfigEnvsAdd_ConfigMapAllKeys(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 || args[0] != "envs" || args[1] != "add" {
			t.Fatalf("expected positional args ['envs','add'], got %v", args[:min(2, len(args))])
		}
		argsMap := argsToMap(args[2:])
		wantValue := "{{ configMap:my-config }}"
		if got, ok := argsMap["--value"]; !ok || got != wantValue {
			t.Fatalf("expected --value=%q, got %q", wantValue, got)
		}
		if _, ok := argsMap["--name"]; ok {
			t.Fatal("expected no --name flag when importing all configmap keys")
		}
		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":          ".",
			"configMapName": "my-config",
		},
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

// TestTool_ConfigEnvsAdd_ValueTakesPrecedence ensures that when both Value and
// SecretName are provided, the explicit Value is used.
func TestTool_ConfigEnvsAdd_ValueTakesPrecedence(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}
		if len(args) < 2 || args[0] != "envs" || args[1] != "add" {
			t.Fatalf("expected positional args ['envs','add'], got %v", args[:min(2, len(args))])
		}
		argsMap := argsToMap(args[2:])
		wantValue := "explicit-value"
		if got, ok := argsMap["--value"]; !ok || got != wantValue {
			t.Fatalf("expected --value=%q, got %q", wantValue, got)
		}
		return []byte("Environment variable added successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"name":       "MY_VAR",
			"value":      "explicit-value",
			"secretName": "ignored-secret",
		},
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

// TestTool_ConfigEnvsAdd_NameWithSecretAllKeys ensures a validation error is returned
// when name is provided alongside an all-keys Secret import (no secretKey).
func TestTool_ConfigEnvsAdd_NameWithSecretAllKeys(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"name":       "MY_VAR",
			"secretName": "my-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when name is set alongside an all-keys Secret import")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_NameWithConfigMapAllKeys ensures a validation error is returned
// when name is provided alongside an all-keys ConfigMap import (no configMapKey).
func TestTool_ConfigEnvsAdd_NameWithConfigMapAllKeys(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":          ".",
			"name":          "MY_VAR",
			"configMapName": "my-config",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when name is set alongside an all-keys ConfigMap import")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_BothSecretAndConfigMap ensures a validation error is returned
// when both secretName and configMapName are provided simultaneously.
func TestTool_ConfigEnvsAdd_BothSecretAndConfigMap(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":          ".",
			"name":          "MY_VAR",
			"secretName":    "my-secret",
			"configMapName": "my-config",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when both secretName and configMapName are provided")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_SecretKeyWithoutSecretName ensures a validation error is returned
// when secretKey is provided without secretName (SEVERE-2).
func TestTool_ConfigEnvsAdd_SecretKeyWithoutSecretName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":      ".",
			"name":      "MY_VAR",
			"secretKey": "MY_KEY",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when secretKey is set without secretName")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_ConfigMapKeyWithoutConfigMapName ensures a validation error is
// returned when configMapKey is provided without configMapName (SEVERE-2).
func TestTool_ConfigEnvsAdd_ConfigMapKeyWithoutConfigMapName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":         ".",
			"name":         "MY_VAR",
			"configMapKey": "MY_KEY",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when configMapKey is set without configMapName")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_InvalidSecretName ensures a validation error is returned when
// secretName contains characters outside the allowed set (SEVERE-3).
func TestTool_ConfigEnvsAdd_InvalidSecretName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"name":       "MY_VAR",
			"secretName": "evil:inject}}",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when secretName contains invalid characters")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_InvalidSecretKey ensures a validation error is returned when
// secretKey contains characters outside the allowed set (SEVERE-3).
func TestTool_ConfigEnvsAdd_InvalidSecretKey(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":       ".",
			"name":       "MY_VAR",
			"secretName": "my-secret",
			"secretKey":  "bad key with spaces",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when secretKey contains invalid characters")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsAdd_InvalidConfigMapName ensures a validation error is returned when
// configMapName contains characters outside the allowed set (SEVERE-3).
func TestTool_ConfigEnvsAdd_InvalidConfigMapName(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		t.Fatal("executor must not be called when input is invalid")
		return nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "config_envs_add",
		Arguments: map[string]any{
			"path":          ".",
			"name":          "MY_VAR",
			"configMapName": "bad}name",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result when configMapName contains invalid characters")
	}
	if executor.ExecuteInvoked {
		t.Fatal("executor must not be invoked when input fails validation")
	}
}

// TestTool_ConfigEnvsList ensures the config_envs_list tool lists environment variables.
func TestTool_ConfigEnvsList(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		// "envs" + "--path" + "." = 3 args
		if len(args) != 3 {
			t.Fatalf("expected 3 args, got %d: %v", len(args), args)
		}
		if args[0] != "envs" {
			t.Fatalf("expected args[0]='envs', got %q", args[0])
		}

		argsMap := argsToMap(args[1:])
		if val, ok := argsMap["--path"]; !ok || val != "." {
			t.Fatalf("expected --path='.', got %q", val)
		}

		return []byte("DATABASE_URL=postgres://localhost\nAPI_KEY=secret\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_list",
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

// TestTool_ConfigEnvsRemove ensures the config_envs_remove tool removes an environment variable.
func TestTool_ConfigEnvsRemove(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path": {"path", "--path", "."},
		"name": {"name", "--name", "API_KEY"},
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "config" {
			t.Fatalf("expected subcommand 'config', got %q", subcommand)
		}

		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
		}

		if args[0] != "envs" {
			t.Fatalf("expected args[0]='envs', got %q", args[0])
		}
		if args[1] != "remove" {
			t.Fatalf("expected args[1]='remove', got %q", args[1])
		}

		validateArgLength(t, args[2:], len(stringFlags), 0)
		validateStringFlags(t, args[2:], stringFlags)

		return []byte("Environment variable removed successfully\n"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, nil)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_remove",
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

// TestTool_ConfigEnvsList_Error ensures the config_envs_list tool propagates executor errors.
func TestTool_ConfigEnvsList_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("list failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_list",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigEnvsAdd_Error ensures the config_envs_add tool propagates executor errors.
func TestTool_ConfigEnvsAdd_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("add failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_add",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}

// TestTool_ConfigEnvsRemove_Error ensures the config_envs_remove tool propagates executor errors.
func TestTool_ConfigEnvsRemove_Error(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("remove failed"), errors.New("executor error")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "config_envs_remove",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
}
