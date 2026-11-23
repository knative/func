package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDefaultExecutor_ExecuteEnv verifies that the extra environment variables
// given to ExecuteEnv reach the subprocess, alongside the inherited
// environment.
func TestDefaultExecutor_ExecuteEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell script")
	}
	t.Setenv("INHERITED_VAR", "inherited")

	script := filepath.Join(t.TempDir(), "printenv.sh")
	body := "#!/bin/sh\necho \"token=$FUNC_GH_ACCESS_TOKEN inherited=$INHERITED_VAR\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	e := defaultExecutor{s: &Server{prefix: script}}
	out, err := e.ExecuteEnv(t.Context(), []string{"FUNC_GH_ACCESS_TOKEN=secret"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if want := "token=secret inherited=inherited"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
