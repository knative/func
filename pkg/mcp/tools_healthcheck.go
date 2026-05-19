package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// healthCheckTool is the MCP tool definition for the healthcheck command.
//
// Migration note (#3771): this tool no longer shells out to the func binary.
// The handler calls pkg/functions directly to verify both server liveness and
// cluster connectivity in a single in-process call.
var healthCheckTool = &mcp.Tool{
	Name:        "healthcheck",
	Title:       "Healthcheck",
	Description: "Checks if the MCP server is running and can reach the Kubernetes cluster. Returns server status, version, and cluster connectivity.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Healthcheck",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

// healthcheckHandler implements the healthcheck tool by calling pkg/functions
// directly instead of shelling out to the func binary.
//
// It performs two checks:
//  1. Server liveness — always passes if this code is executing.
//  2. Cluster connectivity — calls pkg/functions.Client.List() with an empty
//     namespace to verify the Kubernetes API server is reachable. A failure
//     here does not make the overall result an error; it surfaces the reason
//     so the agent can decide whether to proceed with other operations.
func (s *Server) healthcheckHandler(ctx context.Context, _ *mcp.CallToolRequest, _ HealthcheckInput) (_ *mcp.CallToolResult, output HealthcheckOutput, err error) {
	output = HealthcheckOutput{
		Status:  "ok",
		Message: "The MCP server is running",
		Version: version,
	}

	// Verify cluster connectivity by performing a lightweight List via the
	// functions client. An empty namespace means "use the current context
	// namespace". This replaces any subprocess-based environment check and
	// gives the agent richer, typed information about the cluster state.
	_, listErr := s.client.List(ctx, "")
	if listErr != nil {
		output.ClusterConnected = false
		output.ClusterMessage = listErr.Error()
	} else {
		output.ClusterConnected = true
		output.ClusterMessage = "Kubernetes cluster is reachable"
	}

	return
}

// HealthcheckInput defines the input parameters for the healthcheck tool.
// No parameters are required; the tool always checks server + cluster state.
type HealthcheckInput struct{}

// HealthcheckOutput defines the structured output returned by the healthcheck tool.
type HealthcheckOutput struct {
	// Status is always "ok" as long as the MCP server process is running.
	Status string `json:"status" jsonschema:"Status of the MCP server (ok)"`

	// Message is a human-readable description of the server status.
	Message string `json:"message" jsonschema:"Human-readable server status message"`

	// Version is the version of the MCP server.
	Version string `json:"version" jsonschema:"Version of the MCP server"`

	// ClusterConnected reports whether the pkg/functions client could reach
	// the Kubernetes cluster. False means other tools (list, deploy, etc.)
	// will likely fail until the cluster is accessible.
	ClusterConnected bool `json:"clusterConnected" jsonschema:"Whether the Kubernetes cluster is reachable"`

	// ClusterMessage explains the cluster connectivity result. On success it
	// contains a confirmation string; on failure it contains the error reason.
	ClusterMessage string `json:"clusterMessage" jsonschema:"Cluster connectivity detail or error reason"`
}
