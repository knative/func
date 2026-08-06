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
	svc, err := s.requireService()
	if err != nil {
		return result, err
	}
	out, err := svc.Runtimes()
	if err != nil {
		return result, err
	}
	return newSuccessResult("func://languages", "text/plain", []byte(out)), nil
}
