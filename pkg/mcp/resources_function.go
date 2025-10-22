package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var functionStateResource = &mcp.Resource{
	URI:         "func://function",
	Name:        "Current Function State",
	Description: "Current Function configuration (from working directory)",
	MIMEType:    "application/json",
}

func (s *Server) functionStateHandler(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	var (
		uri   = "func://function"
		f     fn.Function
		state []byte
		err   error
	)
	if f, err = fn.NewFunction(""); err != nil {
		return newErrorResult(uri, err), nil
	}
	if !f.Initialized() {
		return newErrorResult(uri, fmt.Errorf("no Function found in current directory")), nil
	}
	if state, err = json.MarshalIndent(f, "", "  "); err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(state),
	}}}, nil
}
