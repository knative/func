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

// TestRegistry_RejectsRelativePath lists every registered tool whose input
// schema has a "path" property and asserts a relative path is rejected.
// A new tool that takes a path and forgets validatePath fails here.
func TestRegistry_RejectsRelativePath(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var checked []string
	for _, tool := range listed.Tools {
		if !schemaHasPath(tool.InputSchema) {
			continue
		}
		checked = append(checked, tool.Name)
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      tool.Name,
			Arguments: relativePathArgs(tool.InputSchema),
		})
		if err != nil {
			t.Fatalf("tool %q: call failed: %v", tool.Name, err)
		}
		if !result.IsError {
			t.Errorf("tool %q: relative path was accepted", tool.Name)
			continue
		}
		got := resultToString(result)
		if !strings.Contains(got, "absolute") {
			t.Errorf("tool %q: expected an absolute-path error, got %q", tool.Name, got)
		}
	}
	if len(checked) == 0 {
		t.Fatal("no registered tools expose a path input; schema discovery is broken")
	}
	for _, name := range []string{
		"config_ci", "config_git_set", "config_git_remove", "invoke", "run", "run_stop",
		"describe", "logs", "build", "deploy", "create",
	} {
		found := false
		for _, c := range checked {
			if c == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %q to be covered by the registry check", name)
		}
	}
	t.Logf("checked %d path-taking tools", len(checked))
}

// relativePathArgs sets path to "." and fills every other required property
// so schema validation reaches the handler's validatePath check.
func relativePathArgs(schema any) map[string]any {
	args := map[string]any{"path": "."}
	raw, err := json.Marshal(schema)
	if err != nil {
		return args
	}
	var doc struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return args
	}
	for _, name := range doc.Required {
		if name == "path" {
			continue
		}
		args[name] = placeholderForSchemaType(doc.Properties[name])
	}
	return args
}

func placeholderForSchemaType(raw json.RawMessage) any {
	var prop struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		return "x"
	}
	types := schemaTypes(prop.Type)
	for _, typ := range types {
		switch typ {
		case "boolean":
			return false
		case "number", "integer":
			return 0
		}
	}
	return "x"
}

func schemaTypes(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaHasPath(schema any) bool {
	raw, err := json.Marshal(schema)
	if err != nil {
		return false
	}
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	_, ok := doc.Properties["path"]
	return ok
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
