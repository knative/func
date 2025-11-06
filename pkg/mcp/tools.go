package mcp

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helpers

func resultToString(result *mcp.CallToolResult) string {
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return fmt.Sprintf("%v", result.Content)
}

// appendStringFlag adds a string flag to args only if the value is non-nil and non-empty.
// This ensures we only pass flags that were explicitly provided by the user.
func appendStringFlag(args []string, flag string, value *string) []string {
	if value != nil && *value != "" {
		return append(args, flag, *value)
	}
	return args
}

// appendBoolFlag adds a boolean flag to args only if the value is non-nil and true.
// This ensures we only pass flags that were explicitly provided by the user.
func appendBoolFlag(args []string, flag string, value *bool) []string {
	if value != nil && *value {
		return append(args, flag)
	}
	return args
}

// ptr returns a pointer to the given value.
// Useful for setting optional annotation fields.
func ptr[T any](v T) *T {
	return &v
}
