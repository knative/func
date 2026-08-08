package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Describe_Args ensures the describe tool executes with all arguments
// passed correctly, and that the structured JSON output from the CLI is parsed
// into DescribeOutput correctly.
func TestTool_Describe_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
	}

	boolFlags := map[string]string{
		"verbose": "--verbose",
	}

	name := "my-function"

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}

		// Expected: 1 positional + 2 string flags (namespace, output) * 2 + 1 bool flag
		// = 1 + 2*2 + 1 = 6 args
		if len(args) != 1+2*2+1 {
			t.Fatalf("expected %d args, got %d: %v", 1+2*2+1, len(args), args)
		}

		if args[0] != name {
			t.Fatalf("expected positional arg %q, got %q", name, args[0])
		}

		rest := args[1:]
		validateStringFlags(t, rest, stringFlags)
		validateBoolFlags(t, rest, boolFlags)
		validateStringFlags(t, rest, map[string]struct {
			jsonKey string
			flag    string
			value   string
		}{
			"output": {"output", "--output", "json"},
		})

		return []byte(`{
			"name": "my-function",
			"namespace": "prod",
			"route": "https://my-function.prod.example.com",
			"routes": ["https://my-function.prod.example.com"],
			"image": "docker.io/alice/my-function:latest",
			"deployer": "knative",
			"labels": {"app": "my-function"},
			"subscriptions": [{"source": "src", "type": "type", "broker": "default"}],
			"revision": "abc123",
			"ready": "true"
		}`), nil
	}

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	inputArgs := buildInputArgs(stringFlags, boolFlags)
	inputArgs["name"] = name

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
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

	var output DescribeOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}

	if output.Name != "my-function" {
		t.Errorf("expected name %q, got %q", "my-function", output.Name)
	}
	if output.Namespace != "prod" {
		t.Errorf("expected namespace %q, got %q", "prod", output.Namespace)
	}
	if output.URL != "https://my-function.prod.example.com" {
		t.Errorf("expected url %q, got %q", "https://my-function.prod.example.com", output.URL)
	}
	if len(output.Routes) != 1 || output.Routes[0] != "https://my-function.prod.example.com" {
		t.Errorf("unexpected routes: %v", output.Routes)
	}
	if output.Image != "docker.io/alice/my-function:latest" {
		t.Errorf("expected image %q, got %q", "docker.io/alice/my-function:latest", output.Image)
	}
	if output.Ready != "true" {
		t.Errorf("expected ready %q, got %q", "true", output.Ready)
	}
	if output.Deployer != "knative" {
		t.Errorf("expected deployer %q, got %q", "knative", output.Deployer)
	}
	if output.Labels["app"] != "my-function" {
		t.Errorf("expected label app=my-function, got %v", output.Labels)
	}
	if len(output.Subscriptions) != 1 || output.Subscriptions[0].Broker != "default" {
		t.Errorf("unexpected subscriptions: %v", output.Subscriptions)
	}
	if output.Revision != "abc123" {
		t.Errorf("expected revision %q, got %q", "abc123", output.Revision)
	}
}

// TestTool_Describe_PathAndNameMutuallyExclusive ensures providing both path
// and name is rejected.
func TestTool_Describe_PathAndNameMutuallyExclusive(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "describe",
		Arguments: map[string]any{
			"path": "/tmp/my-function",
			"name": "my-function",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected describe to be rejected when both path and name are provided")
	}
}

// TestTool_Describe_RequiresPathOrName ensures providing neither path nor name
// is rejected.
func TestTool_Describe_RequiresPathOrName(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected describe to be rejected when neither path nor name is provided")
	}
}

// TestTool_Describe_MalformedJSON ensures the handler returns an error
// (rather than panicking) when the CLI output cannot be parsed as JSON.
func TestTool_Describe_MalformedJSON(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
		Arguments: map[string]any{"name": "my-function"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected describe to return an error result for malformed JSON output")
	}
}

// TestTool_Describe_LeadingStderrWarning ensures the handler still parses
// the JSON payload when the executor's combined stdout+stderr output has
// leading non-JSON noise (e.g. a warning printed to stderr before the CLI
// writes its JSON payload to stdout).
func TestTool_Describe_LeadingStderrWarning(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		return []byte("Warning: unable to determine cluster permissions\n" + `{
			"name": "my-function",
			"namespace": "prod",
			"ready": "true"
		}`), nil
	}

	client, server, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe",
		Arguments: map[string]any{"name": "my-function"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output DescribeOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if output.Name != "my-function" {
		t.Errorf("expected name %q, got %q", "my-function", output.Name)
	}
	if output.Namespace != "prod" {
		t.Errorf("expected namespace %q, got %q", "prod", output.Namespace)
	}
	if output.Ready != "true" {
		t.Errorf("expected ready %q, got %q", "true", output.Ready)
	}
}
