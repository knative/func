package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestCategorizeError verifies that CLI output strings are correctly mapped
// to the appropriate ErrorCategory.
func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantCategory ErrorCategory
	}{
		{
			name:         "registry unauthorized",
			output:       "unauthorized: authentication required",
			wantCategory: RegistryError,
		},
		{
			name:         "registry push denied",
			output:       "failed to push image: access denied",
			wantCategory: RegistryError,
		},
		{
			name:         "cluster unreachable",
			output:       "unable to connect to the server: connection refused",
			wantCategory: ClusterError,
		},
		{
			name:         "kubeconfig error",
			output:       "error loading kubeconfig: no such file or directory",
			wantCategory: ClusterError,
		},
		{
			name:         "build failed",
			output:       "build failed: exit status 1",
			wantCategory: BuildError,
		},
		{
			name:         "buildpack error",
			output:       "ERROR: failed to build: buildpack 'io.buildpacks.go' failed",
			wantCategory: BuildError,
		},
		{
			name:         "missing func yaml",
			output:       "func.yaml not found in current directory",
			wantCategory: ValidationError,
		},
		{
			name:         "not a function directory",
			output:       "no function found in current directory",
			wantCategory: ValidationError,
		},
		{
			name:         "rbac forbidden",
			output:       "Error from server (Forbidden): pods is forbidden: User cannot get resource",
			wantCategory: AuthError,
		},
		{
			name:         "permission denied",
			output:       "permission denied: cannot create deployments",
			wantCategory: AuthError,
		},
		{
			name:         "unknown error",
			output:       "something completely unexpected happened",
			wantCategory: UnknownError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := categorizeError(tt.output)
			if got != tt.wantCategory {
				t.Errorf("categorizeError(%q) = %q, want %q", tt.output, got, tt.wantCategory)
			}
		})
	}
}

// TestStructuredError_Error verifies that StructuredError marshals to valid JSON
// with the expected fields.
func TestStructuredError_Error(t *testing.T) {
	se := newStructuredError("unauthorized: authentication required", nil)


	// Must be valid JSON
	raw := se.Error()
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("StructuredError.Error() is not valid JSON: %v\nGot: %s", err, raw)
	}

	// Must contain errorCategory
	if parsed["errorCategory"] == "" {
		t.Errorf("expected errorCategory in JSON output, got: %s", raw)
	}

	// Must be the correct category
	if parsed["errorCategory"] != string(RegistryError) {
		t.Errorf("expected REGISTRY_ERROR, got: %s", parsed["errorCategory"])
	}

	// Must contain a hint
	if parsed["hint"] == "" {
		t.Errorf("expected hint in JSON output, got: %s", raw)
	}
}

// TestTool_Deploy_StructuredError verifies that when the deploy command fails
// with a registry error, the MCP tool response contains the error category.
func TestTool_Deploy_StructuredError(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand == "deploy" {
			return []byte("failed to push image: unauthorized: authentication required"), fmt.Errorf("exit status 1")
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "deploy",
		Arguments: map[string]any{
			"path": "/some/function",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The tool should return an error result
	if !result.IsError {
		t.Fatalf("expected IsError=true, got false. Content: %v", result.Content)
	}

	content := resultToString(result)

	// Should contain the structured error category
	if !strings.Contains(content, string(RegistryError)) {
		t.Errorf("expected REGISTRY_ERROR in error response, got: %s", content)
	}

	// Should contain the hint
	if !strings.Contains(content, "docker login") {
		t.Errorf("expected docker login hint in error response, got: %s", content)
	}
}

// TestTool_Build_StructuredError verifies that when the build command fails
// with a build error, the MCP tool response contains the error category.
func TestTool_Build_StructuredError(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand == "build" {
			return []byte("ERROR: build failed: buildpack 'io.buildpacks.go' failed with exit status 1"), fmt.Errorf("exit status 1")
		}
		return []byte(""), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "build",
		Arguments: map[string]any{
			"path": "/some/function",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Fatalf("expected IsError=true, got false. Content: %v", result.Content)
	}

	content := resultToString(result)

	if !strings.Contains(content, string(BuildError)) {
		t.Errorf("expected BUILD_ERROR in error response, got: %s", content)
	}
}
