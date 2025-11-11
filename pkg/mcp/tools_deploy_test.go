package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Deploy_Args ensures the deploy tool executes with all arguments passed correctly.
func TestTool_Deploy_Args(t *testing.T) {
	// Test data - defined once and used for both input and validation
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"path":               {"path", "--path", "."},
		"builder":            {"builder", "--builder", "pack"},
		"registry":           {"registry", "--registry", "ghcr.io/user"},
		"image":              {"image", "--image", "ghcr.io/user/my-func:latest"},
		"namespace":          {"namespace", "--namespace", "prod"},
		"gitUrl":             {"gitUrl", "--git-url", "https://github.com/user/repo"},
		"gitBranch":          {"gitBranch", "--git-branch", "main"},
		"gitDir":             {"gitDir", "--git-dir", "functions/my-func"},
		"builderImage":       {"builderImage", "--builder-image", "custom-builder:latest"},
		"domain":             {"domain", "--domain", "example.com"},
		"platform":           {"platform", "--platform", "linux/amd64"},
		"build":              {"build", "--build", "auto"},
		"pvcSize":            {"pvcSize", "--pvc-size", "5Gi"},
		"serviceAccount":     {"serviceAccount", "--service-account", "func-deployer"},
		"remoteStorageClass": {"remoteStorageClass", "--remote-storage-class", "fast"},
	}

	boolFlags := map[string]string{
		"push":             "--push",
		"registryInsecure": "--registry-insecure",
		"buildTimestamp":   "--build-timestamp",
		"remote":           "--remote",
		"verbose":          "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "deploy" {
			t.Fatalf("expected subcommand 'deploy', got %q", subcommand)
		}

		validateArgLength(t, args, len(stringFlags), len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)

		return []byte("Function deployed: https://my-function.example.com\n"), nil
	}

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	// Deploy requires write mode - enable it for this test
	server.readonly = false

	// Build input arguments from test data
	inputArgs := buildInputArgs(stringFlags, boolFlags)

	// Invoke tool with all optional arguments
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "deploy",
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
