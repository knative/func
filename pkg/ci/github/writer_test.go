package github

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestBufferWriter_WriteAndExist exercises the in-memory BufferWriter test double:
// Exist is false on an empty buffer and true after Write.
func TestBufferWriter_WriteAndExist(t *testing.T) {
	bw := NewBufferWriter()

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
	bw := NewBufferWriter()
	path := ".github/workflows/func-deploy.yaml"

	assert.NilError(t, bw.Write(path, []byte("data")))
	assert.Equal(t, bw.Path, path)
}

// TestBufferWriter_Write_ResetsBuffer ensures that consecutive Write calls do
// not accumulate content — each call starts with a fresh buffer.
func TestBufferWriter_Write_ResetsBuffer(t *testing.T) {
	bw := NewBufferWriter()

	assert.NilError(t, bw.Write("p", []byte("first")))
	assert.NilError(t, bw.Write("p", []byte("second")))

	assert.DeepEqual(t, bw.Buffer.Bytes(), []byte("second"))
}
