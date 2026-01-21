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
	Write(path string, raw []byte) error
}

type fileWriter struct{}

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

func NewBufferWriter() *bufferWriter {
	return &bufferWriter{Buffer: &bytes.Buffer{}}
}

func (bw *bufferWriter) Write(path string, raw []byte) error {
	bw.Path = path
	_, err := bw.Buffer.Write(raw)
	return err
}
