package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
	fn "knative.dev/func/pkg/functions"
)

// initTestFunction creates a temporary initialized function and returns its path.
func initTestFunction(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	f := fn.NewFunctionWith(fn.Function{
		Root:        dir,
		Name:        "testfn",
		Runtime:     "go",
		Template:    "http",
		SpecVersion: fn.LastSpecVersion(),
	})
	f.Created = time.Now().UTC()
	data, err := yaml.Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, fn.RunDataDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fn.FunctionFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
