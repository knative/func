package mcp

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// validateArgLength validates that the args slice has the expected length based on
// the number of string flags (2 args each: flag + value) and bool flags (1 arg each).
func validateArgLength(t *testing.T, args []string, stringFlagsCount, boolFlagsCount int) {
	t.Helper()
	expected := stringFlagsCount*2 + boolFlagsCount
	if len(args) != expected {
		t.Fatalf("expected %d args (%d string flags * 2 + %d bool flags), got %d: %v",
			expected, stringFlagsCount, boolFlagsCount, len(args), args)
	}
}

// argsToMap converts a command-line arguments slice into a map for order-independent validation.
// String flags are stored as "--flag" -> "value", boolean flags as "--flag" -> "".
func argsToMap(args []string) map[string]string {
	argsMap := make(map[string]string)
	for i := 0; i < len(args); {
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			// String flag: --flag value
			argsMap[args[i]] = args[i+1]
			i += 2
		} else {
			// Boolean flag: --flag (no value)
			argsMap[args[i]] = ""
			i++
		}
	}
	return argsMap
}

// validateStringFlags checks that all expected string flags are present with correct values.
func validateStringFlags(t *testing.T, args []string, stringFlags map[string]struct {
	jsonKey string
	flag    string
	value   string
}) {
	t.Helper()
	argsMap := argsToMap(args)
	for _, flagInfo := range stringFlags {
		if val, ok := argsMap[flagInfo.flag]; !ok {
			t.Fatalf("missing expected flag %q", flagInfo.flag)
		} else if val != flagInfo.value {
			t.Fatalf("flag %q: expected value %q, got %q", flagInfo.flag, flagInfo.value, val)
		}
	}
}

// validateBoolFlags checks that all expected boolean flags are present.
func validateBoolFlags(t *testing.T, args []string, boolFlags map[string]string) {
	t.Helper()
	argsMap := argsToMap(args)
	for _, flag := range boolFlags {
		if _, ok := argsMap[flag]; !ok {
			t.Fatalf("missing expected flag %q", flag)
		}
	}
}

// TestTool_RejectsRelativePath verifies that every tool handler wired to validatePath
// returns an error result when given a relative path, not just the helper itself.
func TestTool_RejectsRelativePath(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"build", map[string]any{"path": "."}},
		{"create", map[string]any{"path": ".", "language": "go"}},
		{"deploy", map[string]any{"path": "."}},
		{"delete", map[string]any{"path": "."}},
		{"config_envs_list", map[string]any{"path": "."}},
		{"config_envs_add", map[string]any{"path": "."}},
		{"config_envs_remove", map[string]any{"path": "."}},
		{"config_labels_list", map[string]any{"path": "."}},
		{"config_labels_add", map[string]any{"path": "."}},
		{"config_labels_remove", map[string]any{"path": "."}},
		{"config_volumes_list", map[string]any{"path": "."}},
		{"config_volumes_add", map[string]any{"path": "."}},
		{"config_volumes_remove", map[string]any{"path": "."}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			client, _, err := newTestPair(t)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tc.tool,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("tool %q: expected error result for relative path, got success", tc.tool)
			}
		})
	}
}

// TestValidatePath ensures validatePath accepts absolute paths and rejects relative ones.
func TestValidatePath(t *testing.T) {
	if err := validatePath(t.TempDir()); err != nil {
		t.Fatalf("expected no error for absolute path, got: %v", err)
	}
	if err := validatePath("relative/path"); err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
	if err := validatePath("."); err == nil {
		t.Fatal("expected error for '.', got nil")
	}
}

// buildInputArgs constructs the input arguments map for CallTool from test data.
func buildInputArgs(stringFlags map[string]struct {
	jsonKey string
	flag    string
	value   string
}, boolFlags map[string]string) map[string]any {
	inputArgs := make(map[string]any)
	for _, flagInfo := range stringFlags {
		inputArgs[flagInfo.jsonKey] = flagInfo.value
	}
	for key := range boolFlags {
		inputArgs[key] = true
	}
	return inputArgs
}
