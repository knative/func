//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// describe how to set up a Function for a specific runtimes testing lifecycle hooks
type lifecycleRuntime struct {
	builder string // "host", "pack", "s2i"
	// TODO: expand matrix for each deployer
	// deployer string   // "knative","raw","keda"
	srcPath  string   // where to write source in func dir (e.g. "function.go")
	srcExt   string   // extension for testdata lookup (".go", ".py")
	initArgs []string // extra args for func init
}

var allRuntimes = []string{
	"go", "python", "node", "typescript", "rust", "quarkus", "springboot",
}

var lifecycleRuntimes = map[string]*lifecycleRuntime{
	"go": {
		builder:  "host",
		srcPath:  "function.go",
		srcExt:   ".go",
		initArgs: []string{},
	},
	"python": {
		builder:  "host",
		srcPath:  filepath.Join("function", "func.py"),
		srcExt:   ".py",
		initArgs: []string{},
	},
}

// loads testdata/lifecycle/{runtime}/{hook}{extension}
func lifecycleSource(t *testing.T, runtime, hook string, rt *lifecycleRuntime) []byte {
	t.Helper()
	path := filepath.Join(Testdata, "lifecycle", runtime, hook+rt.srcExt)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lifecycleSource: reading %s: %v", path, err)
	}
	return data
}

// getBody performs a single HTTP GET and returns the response body as a string.
func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("getBody: GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("getBody: reading body from %s: %v", url, err)
	}
	return string(body)
}

// extractValue parses KEY=value pairs from a space-separated response body and
// returns the value for the given key, or "" if the key is not present.
// Example: extractValue("ALIVE=true NONCE=12345", "NONCE") == "12345"
func extractValue(body, key string) string {
	for _, field := range strings.Fields(body) {
		if k, v, ok := strings.Cut(field, "="); ok && k == key {
			return v
		}
	}
	return ""
}
