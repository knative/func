package mcp

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resource helpers:

func newErrorResult(uri string, err error) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "text/plain",
				Text:     fmt.Sprintf("%v", err),
			},
		},
	}
}
