package cmd

import (
	"context"
	"os"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestMCP_Start ensures the "mcp start" command starts the MCP server.
func TestMCP_Start(t *testing.T) {
	_ = FromTempDirectory(t)

	server := mock.NewMCPServer()

	cmd := NewMCPCmd(NewTestClient(fn.WithMCPServer(server)))
	cmd.SetArgs([]string{"start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !server.StartInvoked {
		// Indicates a failure of the command to correctly map the request
		// for "mcp start" to an actual invocation of the client's
		// StartMCPServer method, or something more fundamental like failure
		// to register the subcommand, etc.
		t.Fatal("MCP server's start method not invoked")
	}
}

// TestMCP_StartWriteable ensures that the server is started with write
// enabled only when the environment variable FUNC_ENABLE_MCP_WRITE is set.
func TestMCP_StartWriteable(t *testing.T) {
	_ = FromTempDirectory(t)

	// Ensure it defaults to off.
	server := mock.NewMCPServer()
	server.StartFn = func(_ context.Context, writeEnabled bool) error {
		if writeEnabled {
			t.Fatal("MCP server started write-enabled by default")
		}
		return nil
	}
	cmd := NewMCPCmd(NewTestClient(fn.WithMCPServer(server)))
	cmd.SetArgs([]string{"start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure it is set to true on proper truthy value
	_ = os.Setenv("FUNC_ENABLE_MCP_WRITE", "true")
	server = mock.NewMCPServer()
	server.StartFn = func(_ context.Context, writeEnabled bool) error {
		if !writeEnabled {
			t.Fatal("MCP server was not enabled")
		}
		return nil
	}
	cmd = NewMCPCmd(NewTestClient(fn.WithMCPServer(server)))
	cmd.SetArgs([]string{"start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
