package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mcp/mock"
)

func TestResource_FunctionState(t *testing.T) {
	root := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) }) // Change back out before cleanup on Windows
	_ = os.Chdir(root)

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	// Test case 1: Error when no Function exists in working directory
	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "func://function",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	content := result.Contents[0]
	if content.MIMEType != "text/plain" {
		t.Fatalf("expected MIME type 'text/plain' for error, got %q", content.MIMEType)
	}
	if !strings.Contains(content.Text, "no Function found") {
		t.Fatalf("expected error message to contain 'no Function found', got: %s", content.Text)
	}

	// Test case 2: Success when Function exists

	// Initialize a function in the current directory
	f := fn.Function{
		Name:     "my-function",
		Runtime:  "go",
		Registry: "quay.io/user",
		Root:     ".",
	}
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	result, err = client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "func://function",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	content = result.Contents[0]
	if content.MIMEType != "application/json" {
		t.Fatalf("expected MIME type 'application/json', got %q", content.MIMEType)
	}

	// Unmarshal and validate the function state
	var state fn.Function
	if err := json.Unmarshal([]byte(content.Text), &state); err != nil {
		t.Fatalf("failed to unmarshal function state: %v", err)
	}
	if state.Name != "my-function" {
		t.Fatalf("expected Name='my-function', got %q", state.Name)
	}
	if state.Runtime != "go" {
		t.Fatalf("expected Runtime='go', got %q", state.Runtime)
	}
	if state.Created.IsZero() {
		t.Fatal("expected Created timestamp to be set")
	}
}

func TestResource_Languages(t *testing.T) {
	expectedOutput := `go
node
python
etc
`

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "languages" {
			t.Fatalf("expected subcommand 'languages', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args, got %v", args)
		}
		return []byte(expectedOutput), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "func://languages",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.URI != "func://languages" {
		t.Fatalf("expected URI 'func://languages', got %q", content.URI)
	}
	if content.MIMEType != "text/plain" {
		t.Fatalf("expected MIME type 'text/plain', got %q", content.MIMEType)
	}
	if content.Text != expectedOutput {
		t.Fatalf("expected output:\n%s\ngot:\n%s", expectedOutput, content.Text)
	}

	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}

func TestResource_Templates(t *testing.T) {
	expectedOutput := `LANGUAGE     TEMPLATE
go           cloudevents
go           http
node         cloudevents
node         http
python       cloudevents
python       http
etc          etc
`

	executor := mock.NewExecutor()
	executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
		if subcommand != "templates" {
			t.Fatalf("expected subcommand 'templates', got %q", subcommand)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args, got %v", args)
		}
		return []byte(expectedOutput), nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "func://templates",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.URI != "func://templates" {
		t.Fatalf("expected URI 'func://templates', got %q", content.URI)
	}
	if content.MIMEType != "text/plain" {
		t.Fatalf("expected MIME type 'text/plain', got %q", content.MIMEType)
	}
	if content.Text != expectedOutput {
		t.Fatalf("expected output:\n%s\ngot:\n%s", expectedOutput, content.Text)
	}

	if !executor.ExecuteInvoked {
		t.Fatal("executor was not invoked")
	}
}
