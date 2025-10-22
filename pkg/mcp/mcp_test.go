package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStart ensures that the MCP server can be instantiated and started.
func TestStart(t *testing.T) {
	_, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
}

// TestInstructions ensures the instructions.md has been embedded as the
// server's instructions.
func TestInstructions(t *testing.T) {
	// Test both readonly and write modes
	testCases := []struct {
		name     string
		readonly bool
	}{
		{"write_mode", false},
		{"readonly_mode", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, _, err := newTestPairWithReadonly(t, tc.readonly)
			if err != nil {
				t.Fatal(err)
			}

			result := client.InitializeResult()
			if result == nil {
				t.Fatal("InitializeResult is nil")
			}

			if result.Instructions == "" {
				t.Fatal("Instructions are empty")
			}

			if !strings.Contains(result.Instructions, "# Functions MCP Agent Instructions") {
				t.Error("Instructions missing main title")
			}

			// Verify readonly warning is present only in readonly mode
			hasReadonlyWarning := strings.Contains(result.Instructions, "# ⚠️  Read-Only Mode Warning")
			if tc.readonly && !hasReadonlyWarning {
				t.Error("Readonly mode should include readonly warning")
			}
			if !tc.readonly && hasReadonlyWarning {
				t.Error("Write mode should not include readonly warning")
			}
		})
	}
}

// newTestPairWithReadonly returns a ClientSession and Server with the specified readonly mode.
func newTestPairWithReadonly(t *testing.T, readonly bool) (*mcp.ClientSession, *Server, error) {
	t.Helper()
	var (
		errCh                = make(chan error, 1)
		initCh               = make(chan struct{})
		serverTpt, clientTpt = mcp.NewInMemoryTransports()
	)

	// Create a test server with in-memory transport and readonly flag set
	server := New(WithTransport(serverTpt), WithReadonly(readonly))
	server.OnInit = func(ctx context.Context) {
		close(initCh)
	}

	// Start the Server (readonly already set via WithReadonly option)
	go func() {
		errCh <- server.Start(t.Context(), !readonly)
	}()

	// Connect a client to trigger initialization
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)
	session, err := client.Connect(t.Context(), clientTpt, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("client connection failed: %v", err)
	}

	// Wait for init
	select {
	case err = <-errCh:
		return nil, nil, fmt.Errorf("server exited prematurely %v", err)
	case <-t.Context().Done():
		return nil, nil, fmt.Errorf("timeout waiting for server initialization")
	case <-initCh: // Successful start; continue.
	}
	return session, server, nil
}

// newTestPair returns a ClientSession and Server connected over an in-memory transport.
func newTestPair(t *testing.T, options ...Option) (session *mcp.ClientSession, server *Server, err error) {
	t.Helper()
	var (
		errCh                = make(chan error, 1)
		initCh               = make(chan struct{})
		serverTpt, clientTpt = mcp.NewInMemoryTransports()
	)

	oo := []Option{
		WithTransport(serverTpt),
	}
	oo = append(oo, options...)

	// Create a test server with in-memory transport and a channel it signals
	// upon successful initialization.
	server = New(oo...)
	server.OnInit = func(ctx context.Context) {
		close(initCh)
	}

	// Start the Server
	go func() {
		errCh <- server.Start(t.Context(), false)
	}()

	// Connect a client to trigger initialization
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)
	session, err = client.Connect(t.Context(), clientTpt, nil)
	if err != nil {
		err = fmt.Errorf("client connection failed: %v", err)
		return
	}

	// Wait for init
	select {
	case err = <-errCh:
		err = fmt.Errorf("server exited prematurely %v", err)
	case <-t.Context().Done():
		err = fmt.Errorf("timeout waiting for server initialization")
	case <-initCh: // Successful start; continue.
	}
	return
}
