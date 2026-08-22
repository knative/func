package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_List_Args ensures the list tool executes with all arguments passed
// correctly, always forcing --output json regardless of caller input, and
// that the structured JSON output from the CLI is parsed into ListOutput.
func TestTool_List_Args(t *testing.T) {
	stringFlags := map[string]struct {
		jsonKey string
		flag    string
		value   string
	}{
		"namespace": {"namespace", "--namespace", "prod"},
	}

	boolFlags := map[string]string{
		"allNamespaces": "--all-namespaces",
		"verbose":       "--verbose",
	}

	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		if subcommand != "list" {
			t.Fatalf("expected subcommand 'list', got %q", subcommand)
		}

		// len(stringFlags) + 1 for the always-appended --output json
		validateArgLength(t, args, len(stringFlags)+1, len(boolFlags))
		validateStringFlags(t, args, stringFlags)
		validateBoolFlags(t, args, boolFlags)
		validateStringFlags(t, args, map[string]struct {
			jsonKey string
			flag    string
			value   string
		}{
			"output": {"output", "--output", "json"},
		})

		return []byte(`[{"name":"my-func","namespace":"prod","runtime":"go","url":"https://my-func.prod.example.com","ready":"true","deployer":"knative"}]`), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	inputArgs := buildInputArgs(stringFlags, boolFlags)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
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

	var output ListOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(output.Items), output.Items)
	}
	item := output.Items[0]
	if item.Name != "my-func" {
		t.Errorf("expected name %q, got %q", "my-func", item.Name)
	}
	if item.Namespace != "prod" {
		t.Errorf("expected namespace %q, got %q", "prod", item.Namespace)
	}
	if item.Runtime != "go" {
		t.Errorf("expected runtime %q, got %q", "go", item.Runtime)
	}
	if item.URL != "https://my-func.prod.example.com" {
		t.Errorf("expected url %q, got %q", "https://my-func.prod.example.com", item.URL)
	}
	if item.Ready != "true" {
		t.Errorf("expected ready %q, got %q", "true", item.Ready)
	}
	if item.Deployer != "knative" {
		t.Errorf("expected deployer %q, got %q", "knative", item.Deployer)
	}
	if output.Warnings != "" {
		t.Errorf("expected no warnings, got %q", output.Warnings)
	}
}

// TestTool_List_NoFunctionsFound ensures the handler treats the CLI's
// human-readable "no functions found" message (which `func list` prints on
// stdout even under --output json, see cmd/list.go printNoFunctionsFound) as
// a valid empty result rather than a JSON parse failure.
func TestTool_List_NoFunctionsFound(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte(`no functions found in namespace 'prod'

'func list' shows functions that have been deployed to your cluster.`), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output ListOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Items) != 0 {
		t.Errorf("expected no items, got %v", output.Items)
	}
}

// TestTool_List_EmptyStdout ensures a handler that receives completely empty
// stdout (no output at all) also resolves to an empty, non-erroring result.
func TestTool_List_EmptyStdout(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte(""), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output ListOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Items) != 0 {
		t.Errorf("expected no items, got %v", output.Items)
	}
}

// TestTool_List_MalformedJSON ensures the handler returns an error (rather
// than panicking) when the CLI output cannot be parsed as JSON and doesn't
// match the known "no functions found" message.
func TestTool_List_MalformedJSON(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return []byte("not json"), nil, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected list to return an error result for malformed JSON output")
	}
}

// TestTool_List_StderrWarningDoesNotBreakParsing ensures the handler parses
// the JSON payload correctly even when the CLI writes a warning to stderr on
// an otherwise-successful call, and surfaces that warning in the output.
func TestTool_List_StderrWarningDoesNotBreakParsing(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		stdout := []byte(`[{"name":"my-func","namespace":"prod","runtime":"go","url":"https://my-func.prod.example.com","ready":"true","deployer":"knative"}]`)
		stderr := []byte("Warning: cannot connect to keda deployer - skipping\n")
		return stdout, stderr, nil
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var output ListOutput
	if err := unmarshalStructuredContent(result, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(output.Items), output.Items)
	}
	wantWarning := "Warning: cannot connect to keda deployer - skipping"
	if output.Warnings != wantWarning {
		t.Errorf("expected warnings %q, got %q", wantWarning, output.Warnings)
	}
}

// TestTool_List_CLIError ensures a CLI failure (non-zero exit) surfaces as
// an error result including stdout/stderr context.
func TestTool_List_CLIError(t *testing.T) {
	executor := mock.NewExecutor()
	executor.ExecuteSplitFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("Error: cannot connect to cluster"), fmt.Errorf("exit status 1")
	}

	client, _, err := newTestPair(t, WithExecutor(executor))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected list to return an error result when the CLI fails")
	}
}
