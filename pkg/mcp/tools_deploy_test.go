package mcp

import (
	"context"
	"encoding/json"
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

// TestParseDeployedURL verifies URL extraction from both local and remote deploy output.
func TestParseDeployedURL(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "local deploy",
			out:  "✅ Function deployed in namespace \"default\" and exposed at URL: \n   https://my-func.default.example.com\n",
			want: "https://my-func.default.example.com",
		},
		{
			name: "local update",
			out:  "✅ Function updated in namespace \"prod\" and exposed at URL: \n   https://my-func.prod.example.com\n",
			want: "https://my-func.prod.example.com",
		},
		{
			name: "remote pipeline deploy",
			out:  "Function Deployed at https://my-func.remote.example.com\n",
			want: "https://my-func.remote.example.com",
		},
		{
			name: "remote pipeline deploy with surrounding output",
			out:  "Building...\nPushing...\nFunction Deployed at https://my-func.remote.example.com\nDone.\n",
			want: "https://my-func.remote.example.com",
		},
		{
			name: "no url in output",
			out:  "function up-to-date. Force rebuild with --build\n",
			want: "",
		},
		{
			name: "empty output",
			out:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDeployedURL([]byte(tt.out))
			if err != nil {
				t.Fatalf("parseDeployedURL(%q) unexpected error: %v", tt.out, err)
			}
			if got != tt.want {
				t.Errorf("parseDeployedURL(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

// TestTool_Deploy_StructuredOutput verifies that URL is populated from parsed output.
func TestTool_Deploy_StructuredOutput(t *testing.T) {
	const wantURL = "https://my-func.default.example.com"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		out := "✅ Function deployed in namespace \"default\" and exposed at URL: \n   " + wantURL + "\n"
		return []byte(out), nil
	}

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly = false

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "deploy",
		Arguments: map[string]any{"path": t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	raw := resultToString(result)
	var output DeployOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("failed to unmarshal output: %v\nraw: %s", err, raw)
	}
	if output.URL != wantURL {
		t.Errorf("URL = %q, want %q", output.URL, wantURL)
	}
}
