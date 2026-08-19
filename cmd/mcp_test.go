package cmd

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpkg "knative.dev/func/pkg/mcp"
	. "knative.dev/func/pkg/testing"
)

func TestMCP_Start(t *testing.T) {
	_ = FromTempDirectory(t)

	serverTpt, clientTpt := mcp.NewInMemoryTransports()
	testMCPOptions = []mcpkg.Option{mcpkg.WithTransport(serverTpt)}
	t.Cleanup(func() { testMCPOptions = nil })

	cmd := NewMCPCmd(NewTestClient())
	cmd.SetArgs([]string{"start"})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = cmd.ExecuteContext(ctx) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), clientTpt, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	session.Close()
	cancel()
}

func TestMCP_StartWriteable(t *testing.T) {
	_ = FromTempDirectory(t)

	// Readonly mode
	serverTpt, clientTpt := mcp.NewInMemoryTransports()
	testMCPOptions = []mcpkg.Option{mcpkg.WithTransport(serverTpt)}
	t.Cleanup(func() { testMCPOptions = nil })

	cmd := NewMCPCmd(NewTestClient())
	cmd.SetArgs([]string{"start"})
	ctx, cancel := context.WithCancel(t.Context())
	go func() { _ = cmd.ExecuteContext(ctx) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), clientTpt, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	session.Close()
	cancel()

	// Write mode
	t.Setenv("FUNC_ENABLE_MCP_WRITE", "true")
	serverTpt2, clientTpt2 := mcp.NewInMemoryTransports()
	testMCPOptions = []mcpkg.Option{mcpkg.WithTransport(serverTpt2)}

	cmd = NewMCPCmd(NewTestClient())
	cmd.SetArgs([]string{"start"})
	ctx2, cancel2 := context.WithCancel(t.Context())
	go func() { _ = cmd.ExecuteContext(ctx2) }()

	client2 := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session2, err := client2.Connect(t.Context(), clientTpt2, nil)
	if err != nil {
		cancel2()
		t.Fatal(err)
	}
	session2.Close()
	cancel2()
}
