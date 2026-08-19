package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var functionStateResource = &mcp.Resource{
	URI:         "func://function",
	Name:        "Current Function State",
	Description: "Current Function configuration (from working directory)",
	MIMEType:    "application/json",
}

func (s *Server) functionStateHandler(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := "func://function"
	svc, err := s.requireService()
	if err != nil {
		return newErrorResult(uri, err), nil
	}

	// Resource URI may include a path query in future; for now use cwd via empty path
	// which NewFunction resolves to the current working directory.
	state, err := svc.FunctionState("")
	if err != nil {
		return newErrorResult(uri, fmt.Errorf("no Function found: %w", err)), nil
	}

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(state),
	}}}, nil
}
