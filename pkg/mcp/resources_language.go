package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var languagesResource = &mcp.Resource{
	URI:         "func://languages",
	Name:        "Available Languages",
	Description: "List of available language runtimes",
	MIMEType:    "text/plain",
}

func (s *Server) languagesHandler(ctx context.Context, r *mcp.ReadResourceRequest) (result *mcp.ReadResourceResult, err error) {
	out, err := s.executor.Execute(ctx, "languages")
	if err != nil {
		return result, err
	}

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      "func://languages",
		MIMEType: "text/plain",
		Text:     string(out),
	}}}, nil
}
