package cmd

import (
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

// TestMCP_StartWriteable ensures the "mcp start" command executes without
// error in both default (readonly) and write-enabled modes.
// Readonly correctness (instructions reflecting the correct mode) is covered
// by TestInstructions in pkg/mcp.
func TestMCP_StartWriteable(t *testing.T) {
	_ = FromTempDirectory(t)

	// Default: FUNC_ENABLE_MCP_WRITE unset — should start without error.
	server := mock.NewMCPServer()
	cmd := NewMCPCmd(NewTestClient(fn.WithMCPServer(server)))
	cmd.SetArgs([]string{"start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Explicit write mode enabled — should also start without error.
	t.Setenv("FUNC_ENABLE_MCP_WRITE", "true")
	server = mock.NewMCPServer()
	cmd = NewMCPCmd(NewTestClient(fn.WithMCPServer(server)))
	cmd.SetArgs([]string{"start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
