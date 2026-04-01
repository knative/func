package ci_test

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
)

// TestFileWriter_Write_CreatesParentDirs verifies that Write creates any missing
// parent directories before writing the file.
func TestFileWriter_Write_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.yaml")
	content := []byte("hello: world")

	err := ci.DefaultWorkflowWriter.Write(path, content)
	assert.NilError(t, err)

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, content)
}

// TestFileWriter_Write_OverwritesExistingFile verifies that a second Write call
// replaces the previous content rather than appending to it.
func TestFileWriter_Write_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")

	assert.NilError(t, ci.DefaultWorkflowWriter.Write(path, []byte("old")))
	assert.NilError(t, ci.DefaultWorkflowWriter.Write(path, []byte("new")))

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, []byte("new"))
}

// TestFileWriter_Exist_FalseBeforeWrite confirms Exist returns false for a path
// that does not yet exist.
func TestFileWriter_Exist_FalseBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	assert.Assert(t, !ci.DefaultWorkflowWriter.Exist(path))
}

// TestFileWriter_Exist_TrueAfterWrite confirms Exist returns true once the file
// has been written.
func TestFileWriter_Exist_TrueAfterWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")

	assert.NilError(t, ci.DefaultWorkflowWriter.Write(path, []byte("data")))
	assert.Assert(t, ci.DefaultWorkflowWriter.Exist(path))
}

// TestBufferWriter_WriteAndExist exercises the in-memory BufferWriter test double:
// Exist is false on an empty buffer and true after Write.
func TestBufferWriter_WriteAndExist(t *testing.T) {
	bw := ci.NewBufferWriter()

	assert.Assert(t, !bw.Exist("any/path"), "empty buffer should report Exist=false")

	content := []byte("name: deploy")
	err := bw.Write("some/path", content)
	assert.NilError(t, err)

	assert.Assert(t, bw.Exist("any/path"), "non-empty buffer should report Exist=true")
	assert.DeepEqual(t, bw.Buffer.Bytes(), content)
}

// TestBufferWriter_Write_StoresPath checks that Write records the path that was
// passed to it.
func TestBufferWriter_Write_StoresPath(t *testing.T) {
	bw := ci.NewBufferWriter()
	path := ".github/workflows/func-deploy.yaml"

	assert.NilError(t, bw.Write(path, []byte("data")))
	assert.Equal(t, bw.Path, path)
}

// TestBufferWriter_Write_ResetsBuffer ensures that consecutive Write calls do
// not accumulate content — each call starts with a fresh buffer.
func TestBufferWriter_Write_ResetsBuffer(t *testing.T) {
	bw := ci.NewBufferWriter()

	assert.NilError(t, bw.Write("p", []byte("first")))
	assert.NilError(t, bw.Write("p", []byte("second")))

	assert.DeepEqual(t, bw.Buffer.Bytes(), []byte("second"))
}
