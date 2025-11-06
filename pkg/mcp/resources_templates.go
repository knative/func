package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var templatesResource = &mcp.Resource{
	URI:         "func://templates",
	Name:        "Templates",
	Description: "List of available local templates",
	MIMEType:    "text/plain",
}

func (s *Server) templatesHandler(ctx context.Context, r *mcp.ReadResourceRequest) (result *mcp.ReadResourceResult, err error) {
	out, err := s.executor.Execute(ctx, "templates")
	if err != nil {
		return result, err
	}

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      "func://templates",
		MIMEType: "text/plain",
		Text:     string(out),
	}}}, nil
}
