// +build generate

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	archive = "templates.tgz"
	files   = "templates"
)

// on 'go generate' create templates archive (tar -czf templates.tgz templates)
func main() {
	if err := create(archive, files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func create(name, source string) (err error) {
	// Create file on disk
	tarball, err := os.Create(name)
	if err != nil {
		return
	}
	defer tarball.Close()

	// A gzip compressor which writes to the file
	compressor := gzip.NewWriter(tarball)
	defer compressor.Close()

	// A tar writer which writes to gzip compressor
	w := tar.NewWriter(compressor)
	defer w.Close()

	// File walking function which writes tar entries for each file.
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, e error) (err error) {
		if e != nil {
			return e // abort on any failed ReadDir calls.
		}

		// Header
		fi, err := d.Info()
		if err != nil {
			return
		}
		h, err := tar.FileInfoHeader(fi, d.Name())
		if err != nil {
			return
		}
		h.Name = filepath.ToSlash(path)
		if err = w.WriteHeader(h); err != nil {
			return
		}

		// Done if directory
		if d.IsDir() {
			return
		}

		// Data
		data, err := os.Open(path)
		if err != nil {
			return
		}
		_, err = io.Copy(w, data)
		return
	})
}
