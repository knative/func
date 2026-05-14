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

// TestInstructionsStartArgDrivesReadonly covers the cmd/mcp.go flow: New is
// called without WithReadonly, then Start receives writeEnabled. The
// resulting Instructions must reflect Start's argument.
func TestInstructionsStartArgDrivesReadonly(t *testing.T) {
	testCases := []struct {
		name         string
		writeEnabled bool
		wantWarning  bool
	}{
		{name: "readonly", writeEnabled: false, wantWarning: true},
		{name: "write", writeEnabled: true, wantWarning: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, _, err := newTestPairCore(t, !tc.writeEnabled)
			if err != nil {
				t.Fatal(err)
			}

			result := client.InitializeResult()
			if result == nil {
				t.Fatal("InitializeResult is nil")
			}

			hasReadonlyWarning := strings.Contains(result.Instructions, "# ⚠️  Read-Only Mode Warning")
			if hasReadonlyWarning != tc.wantWarning {
				t.Errorf("writeEnabled=%v: got readonly warning=%v, want %v",
					tc.writeEnabled, hasReadonlyWarning, tc.wantWarning)
			}
		})
	}
}

// newTestPairCore is the core logic for creating a ClientSession and Server connected over an in-memory transport.
func newTestPairCore(t *testing.T, readonly bool, options ...Option) (session *mcp.ClientSession, server *Server, err error) {
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
		errCh <- server.Start(t.Context(), !readonly)
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

// newTestPairWithReadonly returns a ClientSession and Server with the specified readonly mode.
func newTestPairWithReadonly(t *testing.T, readonly bool) (*mcp.ClientSession, *Server, error) {
	return newTestPairCore(t, readonly, WithReadonly(readonly))
}

// newTestPair returns a ClientSession and Server connected over an in-memory transport.
func newTestPair(t *testing.T, options ...Option) (session *mcp.ClientSession, server *Server, err error) {
	return newTestPairCore(t, false, options...)
}

// TestBuildArgs verifies that buildArgs constructs the command argument list
// correctly, and in particular that an empty subcommand is never injected.
func TestBuildArgs(t *testing.T) {
	cases := []struct {
		name       string
		prefix     string
		subcommand string
		args       []string
		want       []string
	}{
		{
			name:       "empty subcommand is omitted",
			prefix:     "func",
			subcommand: "",
			args:       []string{"create", "--help"},
			want:       []string{"func", "create", "--help"},
		},
		{
			name:       "non-empty subcommand is included",
			prefix:     "func",
			subcommand: "deploy",
			args:       []string{"--verbose"},
			want:       []string{"func", "deploy", "--verbose"},
		},
		{
			name:       "multi-word prefix is split correctly",
			prefix:     "kn func",
			subcommand: "build",
			args:       []string{"--push"},
			want:       []string{"kn", "func", "build", "--push"},
		},
		{
			name:       "multi-word prefix with empty subcommand",
			prefix:     "kn func",
			subcommand: "",
			args:       []string{"list", "--help"},
			want:       []string{"kn", "func", "list", "--help"},
		},
		{
			name:       "no args",
			prefix:     "func",
			subcommand: "list",
			args:       nil,
			want:       []string{"func", "list"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildArgs(tc.prefix, tc.subcommand, tc.args)
			if len(got) != len(tc.want) {
				t.Fatalf("buildArgs(%q, %q, %v) = %v, want %v", tc.prefix, tc.subcommand, tc.args, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("buildArgs result[%d] = %q, want %q (full: %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}
