package github

import (
	"bytes"
	"os"
	"path/filepath"
)

const (
	dirPerm  = 0755 // u: rwx, g: r-x, o: r-x
	filePerm = 0644 // u: rw-, g: r--, o: r--
)

// DefaultWorkflowWriter is the default implementation for writing workflow files to disk.
var DefaultWorkflowWriter = &fileWriter{}

// fileWriter implements functions.PathWriter
type fileWriter struct{}

// Write writes raw bytes to the specified path, creating directories as needed.
func (fw *fileWriter) Write(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}

	if err := os.WriteFile(path, raw, filePerm); err != nil {
		return err
	}

	return nil
}

func (fw *fileWriter) Exist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// BufferWriter is a test double (fake) that implements functions.PathWriter
// by writing to an in-memory buffer instead of the filesystem.
type BufferWriter struct {
	Path   string
	Buffer *bytes.Buffer
}

// NewBufferWriter creates a new BufferWriter test double.
func NewBufferWriter() *BufferWriter {
	return &BufferWriter{Buffer: &bytes.Buffer{}}
}

// Write is a fake implementation that stores content in the buffer.
func (bw *BufferWriter) Write(path string, raw []byte) error {
	bw.Path = path
	bw.Buffer.Reset()
	_, err := bw.Buffer.Write(raw)
	return err
}

// Exist is a fake implementation that returns true if the buffer has content.
func (bw *BufferWriter) Exist(_ string) bool {
	return bw.Buffer != nil && bw.Buffer.Len() > 0
}
