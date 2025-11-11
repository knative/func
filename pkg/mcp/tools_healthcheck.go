package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var healthCheckTool = &mcp.Tool{
	Name:        "healthcheck",
	Title:       "Healthcheck",
	Description: "Checks if the MCP server is running and responsive",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Healthcheck",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) healthcheckHandler(ctx context.Context, r *mcp.CallToolRequest, input HealthcheckInput) (result *mcp.CallToolResult, output HealthcheckOutput, err error) {
	output = HealthcheckOutput{
		Status:  "ok",
		Message: "The MCP server is running!",
	}
	return
}

// HealthcheckInput defines the input parameters for the healthcheck tool.
// No parameters are required for healthcheck.
type HealthcheckInput struct{}

// HealthcheckOutput defines the structured output returned by the healthcheck tool.
type HealthcheckOutput struct {
	Status  string `json:"status" jsonschema:"Status of the server (ok)"`
	Message string `json:"message" jsonschema:"Healthcheck message"`
}
