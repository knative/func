package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestTool_List_DirectCall verifies the list tool calls pkg/functions.Client.List
// directly (no subprocess) and returns the items as structured output.
func TestTool_List_DirectCall(t *testing.T) {
	lister := mock.NewLister()
	lister.ListFn = func(_ context.Context, namespace string) ([]fn.ListItem, error) {
		if namespace != "prod" {
			t.Fatalf("expected namespace 'prod', got %q", namespace)
		}
		return []fn.ListItem{
			{Name: "my-func", Namespace: "prod", Runtime: "go", Deployer: "knative", URL: "http://my-func.prod.example.com", Ready: "True"},
		}, nil
	}

	fnClient := fn.New(fn.WithListers(lister))
	client, _, err := newTestPair(t, WithClientProvider(func() *fn.Client { return fnClient }))
	if err != nil {
		t.Fatal(err)
	}

	ns := "prod"
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list",
		Arguments: map[string]any{"namespace": ns},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !lister.ListInvoked {
		t.Fatal("lister was not invoked — handler did not call pkg/functions directly")
	}

	// StructuredContent is the canonical channel for #3770/#3771.
	if result.StructuredContent == nil {
		t.Fatal("expected StructuredContent to be populated")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out ListOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal ListOutput: %v", err)
	}
	if len(out.Functions) != 1 || out.Functions[0].Name != "my-func" {
		t.Fatalf("unexpected functions in output: %+v", out.Functions)
	}
}

// TestTool_List_AllNamespaces verifies that AllNamespaces overrides any
// supplied namespace and results in an empty namespace passed to the lister
// (which the lister contract treats as "all namespaces").
func TestTool_List_AllNamespaces(t *testing.T) {
	lister := mock.NewLister()
	lister.ListFn = func(_ context.Context, namespace string) ([]fn.ListItem, error) {
		if namespace != "" {
			t.Fatalf("expected empty namespace when AllNamespaces=true, got %q", namespace)
		}
		return []fn.ListItem{}, nil
	}

	fnClient := fn.New(fn.WithListers(lister))
	client, _, err := newTestPair(t, WithClientProvider(func() *fn.Client { return fnClient }))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "list",
		Arguments: map[string]any{
			"allNamespaces": true,
			"namespace":     "ignored",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !lister.ListInvoked {
		t.Fatal("lister was not invoked")
	}
}

// TestTool_List_RequiresClientProvider ensures the handler returns an error
// (rather than panicking) when no client provider was configured.
func TestTool_List_RequiresClientProvider(t *testing.T) {
	client, _, err := newTestPair(t)
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
		t.Fatal("expected IsError=true when no client provider is configured")
	}
}
