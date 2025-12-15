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

var DefaultWorkflowWriter = &fileWriter{}

type WorkflowWriter interface {
	Write(path string, p []byte) error
}

type fileWriter struct{}

func (fw *fileWriter) Write(path string, p []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}

	if err := os.WriteFile(path, p, filePerm); err != nil {
		return err
	}

	return nil
}

type bufferWriter struct {
	Buffer *bytes.Buffer
}

func NewBufferWriter() *bufferWriter {
	return &bufferWriter{Buffer: &bytes.Buffer{}}
}

func (bw *bufferWriter) Write(_ string, p []byte) error {
	_, err := bw.Buffer.Write(p)
	return err
}
