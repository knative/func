package mcp

import (
	"context"
	"slices"
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
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		if subcommand != "describe" {
			t.Fatalf("expected subcommand 'describe', got %q", subcommand)
		}

		// Expected: 2 string flags (namespace, output) * 2 + 1 bool flag
		// + "--" + name = 2*2 + 1 + 2 = 7 args
		if len(args) != 2*2+1+2 {
			t.Fatalf("expected %d args, got %d: %v", 2*2+1+2, len(args), args)
		}

		// The positional argument (name) comes last, after "--"
		if args[len(args)-2] != "--" || args[len(args)-1] != name {
			t.Fatalf("expected args to end with [-- %s], got %v", name, args)
		}

		rest := args[:len(args)-2]
		validateStringFlags(t, rest, stringFlags)
		validateBoolFlags(t, rest, boolFlags)
		validateStringFlags(t, rest, map[string]struct {
			jsonKey string
			flag    string
			value   string
		}{
			"output": {"output", "--output", "json"},
		})

		// NOTE: fn.Instance.Route has no json tag (unlike its sibling
		// fields), so real `func describe --output json` output emits
		// "Route" capitalized. Using that exact casing here (rather than
		// "route") keeps this test honest about the real CLI wire format.
		return []byte(`{
			"name": "my-function",
			"namespace": "prod",
			"Route": "https://my-function.prod.example.com",
			"routes": ["https://my-function.prod.example.com"],
			"image": "docker.io/alice/my-function:latest",
			"deployer": "knative",
			"labels": {"app": "my-function"},
			"subscriptions": [{"source": "src", "type": "type", "broker": "default"}],
			"middleware": {"version": "1.2.3"},
			"revision": "abc123",
			"ready": "true"
		}`), nil, nil
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
	if !executor.ExecuteSplitInvoked {
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
	if output.Middleware == nil || output.Middleware.Version != "1.2.3" {
		t.Errorf("expected middleware version %q, got %v", "1.2.3", output.Middleware)
	}
	if output.Revision != "abc123" {
		t.Errorf("expected revision %q, got %q", "abc123", output.Revision)
	}
	if output.Warnings != "" {
		t.Errorf("expected no warnings, got %q", output.Warnings)
	}
}

// TestTool_Describe_NoMiddleware ensures the middleware field is omitted
// (left nil) when the CLI reports no middleware version, rather than
// surfacing a zero-value struct.
func TestTool_Describe_NoMiddleware(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte(`{"name": "my-function"}`), nil, nil
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
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output DescribeOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if output.Middleware != nil {
		t.Errorf("expected nil middleware, got %v", output.Middleware)
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
			"path": testAbsPath("my-function"),
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

// TestTool_Describe_PathAndNamespaceRejected ensures providing both 'path'
// and 'namespace' is rejected: path mode determines the namespace from the
// Function's own deploy identity (func.yaml), and the CLI itself rejects a
// separate --namespace in that mode.
func TestTool_Describe_PathAndNamespaceRejected(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		t.Fatal("executor should not be invoked when path+namespace validation fails")
		return nil, nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "describe",
		Arguments: map[string]any{
			"path":      testAbsPath("my-function"),
			"namespace": "prod",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected describe to be rejected when both path and namespace are provided")
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
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte("not json"), nil, nil
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

// TestTool_Describe_StderrWarningDoesNotBreakParsing ensures the handler
// parses the JSON payload correctly even when the CLI writes a warning to
// stderr on an otherwise-successful call (e.g. the knative describer's
// permission warnings, see pkg/knative/describer.go). This is exactly the
// scenario ExecuteSplit exists for: stdout and stderr are captured into
// independent buffers by the executor, so stderr content can never corrupt
// the JSON parse regardless of how the two streams would have interleaved
// under CombinedOutput.
func TestTool_Describe_StderrWarningDoesNotBreakParsing(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		stdout := []byte(`{
			"name": "my-function",
			"namespace": "prod",
			"ready": "true"
		}`)
		stderr := []byte("Warning: cannot list eventing triggers (permission denied) - skipping\n")
		return stdout, stderr, nil
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
	wantWarning := "Warning: cannot list eventing triggers (permission denied) - skipping"
	if output.Warnings != wantWarning {
		t.Errorf("expected warnings %q, got %q", wantWarning, output.Warnings)
	}
}

// TestTool_Describe_InvalidName ensures the describe tool rejects a name that
// is not a valid Function name, before anything is executed. Without this, a
// name such as "--namespace=kube-system" would be parsed by the CLI as a flag.
func TestTool_Describe_InvalidName(t *testing.T) {
	for _, name := range []string{"--all", "-h", "--namespace=kube-system", "My_Function", ""} {
		t.Run(name, func(t *testing.T) {
			executor := mock.NewExecutor()
			client, _, err := newTestPair(t, WithExecutor(executor))
			if err != nil {
				t.Fatal(err)
			}

			result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "describe",
				Arguments: map[string]any{"name": name},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("expected error result for name %q, got success", name)
			}
			if executor.ExecuteSplitInvoked {
				t.Fatalf("executor was invoked for invalid name %q", name)
			}
		})
	}
}

// TestDescribeInput_Args_NameAfterDoubleDash ensures the name is always passed
// after "--", so the CLI can never parse it as a flag, even if handler
// validation were bypassed.
func TestDescribeInput_Args_NameAfterDoubleDash(t *testing.T) {
	args := DescribeInput{Name: ptr("--help")}.Args()
	want := []string{"--output", "json", "--", "--help"}
	if !slices.Equal(args, want) {
		t.Fatalf("expected %v, got %v", want, args)
	}
}
