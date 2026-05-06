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
	result, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{
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

	result, err = client.ReadResource(t.Context(), &mcp.ReadResourceParams{
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

// TestResource_Help verifies that every help resource handler invokes the
// executor with an empty subcommand and the correct command arguments ending
// with "--help". This is the regression test for the bug where the empty
// subcommand was unconditionally appended, injecting a spurious "" argument
// into every help command.
func TestResource_Help(t *testing.T) {
	cases := []struct {
		name     string
		uri      string
		wantArgs []string // arguments expected after the empty subcommand
	}{
		{
			name:     "root help",
			uri:      "func://help",
			wantArgs: []string{"--help"},
		},
		{
			name:     "create help",
			uri:      "func://help/create",
			wantArgs: []string{"create", "--help"},
		},
		{
			name:     "build help",
			uri:      "func://help/build",
			wantArgs: []string{"build", "--help"},
		},
		{
			name:     "deploy help",
			uri:      "func://help/deploy",
			wantArgs: []string{"deploy", "--help"},
		},
		{
			name:     "list help",
			uri:      "func://help/list",
			wantArgs: []string{"list", "--help"},
		},
		{
			name:     "config volumes help",
			uri:      "func://help/config/volumes",
			wantArgs: []string{"config", "volumes", "--help"},
		},
		{
			name:     "config volumes add help",
			uri:      "func://help/config/volumes/add",
			wantArgs: []string{"config", "volumes", "add", "--help"},
		},
		{
			name:     "config labels help",
			uri:      "func://help/config/labels",
			wantArgs: []string{"config", "labels", "--help"},
		},
		{
			name:     "config labels add help",
			uri:      "func://help/config/labels/add",
			wantArgs: []string{"config", "labels", "add", "--help"},
		},
		{
			name:     "config envs help",
			uri:      "func://help/config/envs",
			wantArgs: []string{"config", "envs", "--help"},
		},
		{
			name:     "config envs add help",
			uri:      "func://help/config/envs/add",
			wantArgs: []string{"config", "envs", "add", "--help"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const helpOutput = "Usage: func ...\n"

			executor := mock.NewExecutor()
			executor.ExecuteFn = func(_ context.Context, subcommand string, args ...string) ([]byte, error) {
				// subcommand must always be empty: help resources encode the
				// command path entirely in args, not in the subcommand field.
				if subcommand != "" {
					t.Errorf("expected empty subcommand, got %q", subcommand)
				}
				// No arg may be an empty string — that was the original bug.
				for i, a := range args {
					if a == "" {
						t.Errorf("args[%d] is an empty string (full args: %v)", i, args)
					}
				}
				if len(args) != len(tc.wantArgs) {
					t.Fatalf("expected args %v, got %v", tc.wantArgs, args)
				}
				for i := range args {
					if args[i] != tc.wantArgs[i] {
						t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
					}
				}
				return []byte(helpOutput), nil
			}

			client, _, err := newTestPair(t, WithExecutor(executor))
			if err != nil {
				t.Fatal(err)
			}

			result, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: tc.uri})
			if err != nil {
				t.Fatalf("ReadResource(%q): %v", tc.uri, err)
			}

			if len(result.Contents) != 1 {
				t.Fatalf("expected 1 content, got %d", len(result.Contents))
			}
			if result.Contents[0].Text != helpOutput {
				t.Errorf("expected output %q, got %q", helpOutput, result.Contents[0].Text)
			}
			if !executor.ExecuteInvoked {
				t.Fatal("executor was not invoked")
			}
		})
	}
}

func TestResource_Listings(t *testing.T) {
	cases := []struct {
		name           string
		uri            string
		subcommand     string
		expectedOutput string
	}{
		{
			name:       "languages",
			uri:        "func://languages",
			subcommand: "languages",
			expectedOutput: `go
node
python
etc
`,
		},
		{
			name:       "templates",
			uri:        "func://templates",
			subcommand: "templates",
			expectedOutput: `LANGUAGE     TEMPLATE
go           cloudevents
go           http
node         cloudevents
node         http
python       cloudevents
python       http
etc          etc
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executor := mock.NewExecutor()
			executor.ExecuteFn = func(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
				if subcommand != tc.subcommand {
					t.Fatalf("expected subcommand %q, got %q", tc.subcommand, subcommand)
				}
				if len(args) != 0 {
					t.Fatalf("expected no args, got %v", args)
				}
				return []byte(tc.expectedOutput), nil
			}

			client, _, err := newTestPair(t, WithExecutor(executor))
			if err != nil {
				t.Fatal(err)
			}

			result, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{
				URI: tc.uri,
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(result.Contents) != 1 {
				t.Fatalf("expected 1 content, got %d", len(result.Contents))
			}

			content := result.Contents[0]
			if content.URI != tc.uri {
				t.Fatalf("expected URI %q, got %q", tc.uri, content.URI)
			}
			if content.MIMEType != "text/plain" {
				t.Fatalf("expected MIME type 'text/plain', got %q", content.MIMEType)
			}
			if content.Text != tc.expectedOutput {
				t.Fatalf("expected output:\n%s\ngot:\n%s", tc.expectedOutput, content.Text)
			}

			if !executor.ExecuteInvoked {
				t.Fatal("executor was not invoked")
			}
		})
	}
}
