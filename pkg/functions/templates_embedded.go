package functions

import (
	"archive/zip"
	"bytes"

	"knative.dev/func/generate"
	"knative.dev/func/pkg/filesystem"
)

//go:generate go run ../../generate/templates/main.go

func newEmbeddedTemplatesFS() filesystem.Filesystem {
	archive, err := zip.NewReader(bytes.NewReader(generate.TemplatesZip), int64(len(generate.TemplatesZip)))
	if err != nil {
		panic(err)
	}
	return filesystem.NewZipFS(archive)
}

var EmbeddedTemplatesFS filesystem.Filesystem = newEmbeddedTemplatesFS()
