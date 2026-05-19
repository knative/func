package cmd

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mcp"
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

// passthroughClientFactory is a ClientFactory that applies all options
// provided at invocation time (i.e. by the command itself) to the client,
// rather than substituting its own. This allows runMCPStart to construct
// and inject its own mcp.Server, which the test then exercises directly.
func passthroughClientFactory(_ ClientConfig, options ...fn.Option) (*fn.Client, func()) {
	return fn.New(options...), func() {}
}

// TestMCP_StartWriteable ensures the server is started with write enabled only
// when FUNC_ENABLE_MCP_WRITE is set. Tool capability is verified behaviorally
// by connecting a real MCP client over an in-memory transport to the server
// that runMCPStart constructs.
func TestMCP_StartWriteable(t *testing.T) {
	_ = FromTempDirectory(t)

	tests := []struct {
		name       string
		envVal     string
		wantDeploy bool
		wantDelete bool
	}{
		{
			name:       "readonly by default",
			wantDeploy: false,
			wantDelete: false,
		},
		{
			name:       "writeable when FUNC_ENABLE_MCP_WRITE=true",
			envVal:     "true",
			wantDeploy: true,
			wantDelete: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envVal != "" {
				t.Setenv("FUNC_ENABLE_MCP_WRITE", tc.envVal)
			}

			// In-memory transport pair: the server side is injected into the
			// mcp.Server that runMCPStart constructs; the client side lets us
			// verify tool capabilities without stdio or network I/O.
			serverTpt, clientTpt := sdk.NewInMemoryTransports()

			// runMCPStart blocks until its transport is closed. Run it in a
			// goroutine and collect any unexpected errors.
			errCh := make(chan error, 1)
			go func() {
				root := NewMCPCmd(passthroughClientFactory)
				root.SetContext(t.Context())
				errCh <- runMCPStart(root, nil, passthroughClientFactory,
					mcp.WithTransport(serverTpt))
			}()

			// Connect a test client. Connect() completes the MCP protocol
			// handshake synchronously before returning, so the tool list is
			// immediately available.
			sdkClient := sdk.NewClient(&sdk.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			}, nil)
			session, err := sdkClient.Connect(t.Context(), clientTpt, nil)
			if err != nil {
				// Surface a server-side error if it caused the connect failure.
				select {
				case serverErr := <-errCh:
					t.Fatalf("server error: %v (connect error: %v)", serverErr, err)
				default:
				}
				t.Fatalf("failed to connect MCP client: %v", err)
			}

			res, err := session.ListTools(t.Context(), &sdk.ListToolsParams{})
			if err != nil {
				t.Fatalf("failed to list tools: %v", err)
			}

			exposed := make(map[string]bool, len(res.Tools))
			for _, tool := range res.Tools {
				exposed[tool.Name] = true
			}

			if exposed["deploy"] != tc.wantDeploy {
				t.Errorf("deploy tool exposed=%v, want %v", exposed["deploy"], tc.wantDeploy)
			}
			if exposed["delete"] != tc.wantDelete {
				t.Errorf("delete tool exposed=%v, want %v", exposed["delete"], tc.wantDelete)
			}
		})
	}
}
