package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/mcp/mock"
)

// TestTool_Run_Args ensures the run tool passes the correct arguments to the
// process starter and returns pid/url from the started process.
func TestTool_Run_Args(t *testing.T) {
	path := testAbsPath("my-func")

	starter := mock.NewProcessStarter()
	starter.StartFn = func(ctx context.Context, subcommand string, args ...string) (int, string, string, func() error, error) {
		if subcommand != "run" {
			t.Fatalf("expected subcommand 'run', got %q", subcommand)
		}

		// path, --json, registry, build, port -> 3 string flags (path, registry, address) * 2 + 1 bool-ish (--json) + 1 (--build=true, standalone flag)
		wantArgs := map[string]string{
			"--path":     path,
			"--registry": "ghcr.io/user",
			"--address":  "127.0.0.1:9090",
		}
		got := argsToMap(args)
		for flag, val := range wantArgs {
			if got[flag] != val {
				t.Fatalf("expected %s=%s, got args %v", flag, val, args)
			}
		}
		if _, ok := got["--json"]; !ok {
			t.Fatalf("expected --json flag, got args %v", args)
		}
		if _, ok := got["--build=true"]; !ok {
			t.Fatalf("expected --build=true flag, got args %v", args)
		}

		return 4242, "127.0.0.1", "9090", func() error { return nil }, nil
	}

	client, server, err := newTestPair(t, WithProcessStarter(starter))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	port := 9090
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "run",
		Arguments: map[string]any{
			"path":     path,
			"registry": "ghcr.io/user",
			"build":    true,
			"port":     port,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !starter.StartInvoked {
		t.Fatal("process starter was not invoked")
	}

	text := resultToString(result)
	if wantURL := "http://127.0.0.1:9090"; !strings.Contains(text, wantURL) {
		t.Fatalf("expected result to contain %q, got %q", wantURL, text)
	}
	if !strings.Contains(text, "4242") {
		t.Fatalf("expected result to contain pid 4242, got %q", text)
	}
}

// TestTool_Run_AllowedInReadonly ensures the run tool is NOT gated by
// readonly mode: it is a local-only operation (no cluster mutation), unlike
// deploy/delete.
func TestTool_Run_AllowedInReadonly(t *testing.T) {
	starter := mock.NewProcessStarter()
	starter.StartFn = func(ctx context.Context, subcommand string, args ...string) (int, string, string, func() error, error) {
		return 1, "127.0.0.1", "8080", func() error { return nil }, nil
	}

	client, _, err := newTestPairCore(t, true, WithProcessStarter(starter))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run",
		Arguments: map[string]any{"path": testAbsPath("my-func")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected run to be allowed in readonly mode, got error: %v", result)
	}
}

// TestTool_Run_Duplicate ensures a second run for the same path is rejected
// while the first is still active.
func TestTool_Run_Duplicate(t *testing.T) {
	starter := mock.NewProcessStarter()
	starter.StartFn = func(ctx context.Context, subcommand string, args ...string) (int, string, string, func() error, error) {
		return 1, "127.0.0.1", "8080", func() error { return nil }, nil
	}

	client, server, err := newTestPair(t, WithProcessStarter(starter))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	params := &mcp.CallToolParams{Name: "run", Arguments: map[string]any{"path": testAbsPath("dup-func")}}

	result, err := client.CallTool(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error on first run: %v", result)
	}

	result, err = client.CallTool(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected second run for the same path to be rejected")
	}
}

// TestTool_Run_StartError ensures a failure from the process starter is
// surfaced as a tool error.
func TestTool_Run_StartError(t *testing.T) {
	starter := mock.NewProcessStarter()
	starter.StartFn = func(ctx context.Context, subcommand string, args ...string) (int, string, string, func() error, error) {
		return 0, "", "", nil, fmt.Errorf("boom")
	}

	client, server, err := newTestPair(t, WithProcessStarter(starter))
	if err != nil {
		t.Fatal(err)
	}
	server.readonly.Store(false)

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run",
		Arguments: map[string]any{"path": testAbsPath("err-func")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected run to surface the process starter's error")
	}
}
