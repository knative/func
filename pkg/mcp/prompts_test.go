package mcp

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrompt_Listed(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, p := range result.Prompts {
		if p.Name == "func-workflow" {
			found = true
			if p.Description == "" {
				t.Error("expected non-empty description")
			}
			break
		}
	}
	if !found {
		t.Fatal("func-workflow prompt not found in listing")
	}
}

func TestPrompt_Get(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "func-workflow",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	msg := result.Messages[0]
	if msg.Role != "user" {
		t.Fatalf("expected role 'user', got %q", msg.Role)
	}
	tc, ok := msg.Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", msg.Content)
	}
	if !strings.Contains(tc.Text, "create") {
		t.Error("expected prompt text to mention create")
	}
	if !strings.Contains(tc.Text, "deploy") {
		t.Error("expected prompt text to mention deploy")
	}
}

func TestPrompt_GetWithLanguage(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "func-workflow",
		Arguments: map[string]string{"language": "go"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	tc, ok := result.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Messages[0].Content)
	}
	if !strings.Contains(tc.Text, "go") {
		t.Error("expected prompt text to contain the language 'go'")
	}
}
