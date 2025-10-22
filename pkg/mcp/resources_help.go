package mcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newHelpResource returns a resource which will output the --help text
// of a given command.
func newHelpResource(s *Server, name string, desc string, cmd ...string) (resource *mcp.Resource, handler mcp.ResourceHandler) {
	// URI in format "func://help/command/subcommand"
	uri := strings.Join(append([]string{"func://help"}, cmd...), "/")

	resource = &mcp.Resource{
		URI:         uri,
		Name:        name,
		Description: desc,
		MIMEType:    "text/plain",
	}

	handler = func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		out, err := s.executor.Execute(ctx, "", append(cmd, "--help")...)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "text/plain",
			Text:     string(out),
		}}}, nil
	}
	return
}
