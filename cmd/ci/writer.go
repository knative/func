package ci

import (
	"bytes"
	"os"
	"path/filepath"
)

const (
	dirPerm  = 0755 // o: rwx, g|u: r-x
	filePerm = 0644 // o: rw,  g|u: r
)

// DefaultWorkflowWriter is the default implementation for writing workflow files to disk.
var DefaultWorkflowWriter = &fileWriter{}

// WorkflowWriter defines the interface for writing workflow files.
type WorkflowWriter interface {
	Write(path string, raw []byte) error
}

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

type bufferWriter struct {
	Path   string
	Buffer *bytes.Buffer
}

// NewBufferWriter creates a new bufferWriter for testing purposes.
func NewBufferWriter() *bufferWriter {
	return &bufferWriter{Buffer: &bytes.Buffer{}}
}

// Write stores the path and writes raw bytes to the internal buffer.
func (bw *bufferWriter) Write(path string, raw []byte) error {
	bw.Path = path
	_, err := bw.Buffer.Write(raw)
	return err
}
