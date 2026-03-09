package github

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
)

func TestGitHubWorkflow_Export(t *testing.T) {
	// GIVEN
	viper.Set("platform", "github")
	t.Cleanup(func() { viper.Reset() })
	bufferWriter := NewBufferWriter()

	// WHEN
	cfg := Config{
		FnRoot:    "path/to/function",
		FnRuntime: "go",
	}

	gw, workflowErr := newGitHubWorkflow(cfg, &bytes.Buffer{})
	assert.NilError(t, workflowErr, "unexpected error when creating GitHub Workflow")

	exportErr := gw.Export(cfg.FnGitHubWorkflowFilepath(), bufferWriter, true, &bytes.Buffer{})

	// THEN
	assert.NilError(t, exportErr, "unexpected error when exporting GitHub Workflow")
	assert.Assert(t, strings.Contains(bufferWriter.Buffer.String(), gw.Name))
}
