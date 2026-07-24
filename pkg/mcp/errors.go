package mcp

import "errors"

var errReadOnlyMode = errors.New("the server is currently in read-only mode; to enable write operations, set FUNC_ENABLE_MCP_WRITE in the server environment and restart the server")
