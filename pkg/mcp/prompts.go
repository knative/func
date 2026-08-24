package mcp

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// prompt helpers:

// newUserPromptResult returns a GetPromptResult carrying a single user-role
// text message. Every func prompt is a single, self-contained instruction
// block for the agent, so this is the only shape currently needed.
func newUserPromptResult(description, text string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Description: description,
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			},
		},
	}
}

// rawPromptArg returns the named argument from the request verbatim, for
// values which are case-sensitive (such as a container registry). Returns ""
// when the argument was not provided.
func rawPromptArg(r *mcp.GetPromptRequest, name string) string {
	if r == nil || r.Params == nil {
		return ""
	}
	return r.Params.Arguments[name]
}

// promptArg returns the named argument from the request, normalized by
// trimming surrounding whitespace and lowercasing. Prompt arguments arrive as
// free-form strings typed by a human (or filled in by an agent), so "  Local "
// and "local" must be treated as the same value. Use this only for arguments
// whose valid values are enumerated by the prompt itself; values passed
// through to func (a runtime, a template, a registry) must keep their case
// and so use rawPromptArg. Returns "" when the argument was not provided.
func promptArg(r *mcp.GetPromptRequest, name string) string {
	return strings.ToLower(strings.TrimSpace(rawPromptArg(r, name)))
}

// validateChoice ensures value is one of allowed, returning an error naming
// the offending argument and the valid set. An empty value is always
// accepted: prompt arguments are optional by design, and an omitted one means
// "the agent should ask the user", not "invalid".
func validateChoice(name, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("invalid %q argument %q: must be one of %s",
		name, value, strings.Join(allowed, ", "))
}
