package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// unmarshalStructuredContent decodes a CallToolResult's StructuredContent
// (which may arrive as json.RawMessage or as an already-decoded map) into out.
func unmarshalStructuredContent(result *mcp.CallToolResult, out any) error {
	switch sc := result.StructuredContent.(type) {
	case json.RawMessage:
		return json.Unmarshal(sc, out)
	case nil:
		return fmt.Errorf("result has no structured content")
	default:
		b, err := json.Marshal(sc)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
}

// testAbsPath returns a platform-native absolute path for name, suitable for
// tools which require an absolute path. A POSIX literal such as "/tmp/f" can
// not be used directly because it is not absolute on Windows.
func testAbsPath(name string) string {
	return filepath.Join(os.TempDir(), name)
}

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
