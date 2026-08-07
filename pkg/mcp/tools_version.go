package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var versionTool = &mcp.Tool{
	Name:        "version",
	Title:       "Version",
	Description: "Reports the version of the func client binary, so agents can gate feature usage on version support.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Version",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) versionHandler(ctx context.Context, r *mcp.CallToolRequest, input VersionInput) (result *mcp.CallToolResult, output VersionOutput, err error) {
	out, err := s.executor.Execute(ctx, "version", "--output", "json")
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}

	// raw mirrors only the fields of cmd.Version's JSON output that we need
	// (see cmd/version.go); importing cmd directly would create an import
	// cycle since cmd/mcp.go imports this package.
	var raw struct {
		Vers string `json:"version,omitempty"`
		Hash string `json:"commit,omitempty"`
	}
	if err = json.Unmarshal(out, &raw); err != nil {
		err = fmt.Errorf("error parsing version output: %w\n%s", err, string(out))
		return
	}

	output = VersionOutput{
		Version:     raw.Vers,
		GitRevision: raw.Hash,
	}
	return
}

// VersionInput defines the input parameters for the version tool.
// No parameters are required for version.
type VersionInput struct{}

// VersionOutput defines the structured output returned by the version tool.
type VersionOutput struct {
	Version     string `json:"version" jsonschema:"Version of the func client binary"`
	GitRevision string `json:"gitRevision,omitempty" jsonschema:"Git commit hash the binary was built from, if available"`
}
