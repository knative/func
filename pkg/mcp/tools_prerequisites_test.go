package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_CheckPrerequisites verifies that when all external commands succeed
// and a Function with a registry is present, all checks pass and ready is true.
func TestTool_CheckPrerequisites(t *testing.T) {
	// Set up a temp dir with a valid func.yaml so the registry check passes
	root := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(root)

	// Initialize a function with a registry configured
	f := fn.Function{
		Name:     "test-func",
		Runtime:  "go",
		Registry: "docker.io/testuser",
		Root:     ".",
	}
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	executor := mock.NewExecutor()

	// Mock for func subcommands (Execute)
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand == "version" {
			return []byte("v1.25.7"), nil
		}
		return []byte(""), nil
	}

	// Mock for raw commands (ExecuteRaw)
	executor.ExecuteRawFn = func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
		switch cmd {
		case "docker":
			return []byte("Docker version 27.0.0"), nil
		case "kubectl":
			if len(args) > 0 && args[0] == "cluster-info" {
				return []byte("Kubernetes control plane is running at https://127.0.0.1:6443"), nil
			}
			if len(args) > 2 && args[0] == "get" && args[2] == "services.serving.knative.dev" {
				return []byte("customresourcedefinition.apiextensions.k8s.io/services.serving.knative.dev reconciled"), nil
			}
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_prerequisites",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	content := resultToString(result)
	if !strings.Contains(content, "v1.25.7") {
		t.Errorf("expected output to contain func version, got: %s", content)
	}
	if !strings.Contains(content, "Docker version 27.0.0") {
		t.Errorf("expected output to contain docker info, got: %s", content)
	}
	if !strings.Contains(content, "Kubernetes control plane") {
		t.Errorf("expected output to contain cluster info, got: %s", content)
	}
	if !strings.Contains(content, "services.serving.knative.dev") {
		t.Errorf("expected output to contain knative check, got: %s", content)
	}
	if !strings.Contains(content, "docker.io/testuser") {
		t.Errorf("expected output to contain registry, got: %s", content)
	}
	if !strings.Contains(content, `"ready"`) {
		t.Errorf("expected output to contain ready field, got: %s", content)
	}
}

// TestTool_CheckPrerequisites_DockerFailure verifies that when Docker is not
// running, the check reports an error with guidance.
func TestTool_CheckPrerequisites_DockerFailure(t *testing.T) {
	root := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(root)

	executor := mock.NewExecutor()

	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("v1.25.7"), nil
	}

	executor.ExecuteRawFn = func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
		if cmd == "docker" {
			return []byte("Cannot connect to the Docker daemon"), fmt.Errorf("exit status 1")
		}
		// kubectl checks pass
		if cmd == "kubectl" {
			return []byte("OK"), nil
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_prerequisites",
	})
	if err != nil {
		t.Fatal(err)
	}

	content := resultToString(result)

	// Should report docker error
	if !strings.Contains(content, "Cannot connect to the Docker daemon") {
		t.Errorf("expected docker error in output, got: %s", content)
	}
	// Should include guidance
	if !strings.Contains(content, "Docker Desktop") {
		t.Errorf("expected docker guidance in output, got: %s", content)
	}
}

// TestTool_CheckPrerequisites_NoRegistry verifies that when no registry is
// configured, the check reports a warning with guidance.
func TestTool_CheckPrerequisites_NoRegistry(t *testing.T) {
	// Use a temp dir with NO func.yaml
	root := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(root)

	executor := mock.NewExecutor()

	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("v1.25.7"), nil
	}

	executor.ExecuteRawFn = func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
		return []byte("OK"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_prerequisites",
	})
	if err != nil {
		t.Fatal(err)
	}

	content := resultToString(result)

	// Should report registry warning
	if !strings.Contains(content, "No container registry configured") {
		t.Errorf("expected registry warning in output, got: %s", content)
	}
	// Should include registry guidance
	if !strings.Contains(content, "func config registry") {
		t.Errorf("expected registry guidance in output, got: %s", content)
	}
}
