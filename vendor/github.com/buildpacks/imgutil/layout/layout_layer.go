package layout

import (
	"bytes"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type notExistsLayer struct {
	v1.Layer
	diffID v1.Hash
}

func (l *notExistsLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte{})), nil
}

func (l *notExistsLayer) DiffID() (v1.Hash, error) {
	return l.diffID, nil
}

func (l *notExistsLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte{})), nil
}
