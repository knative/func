package tekton

import (
	"archive/tar"
	"errors"
	"io"
	"path/filepath"
	"testing"

	fn "knative.dev/func"
)

func TestSourcesAsTarStream(t *testing.T) {
	rc := sourcesAsTarStream(fn.Function{Root: filepath.Join("testData", "fn-src")})
	t.Cleanup(func() { _ = rc.Close() })

	var helloTxtContent []byte
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if hdr.Name == "source/bin/release/a.out" {
			t.Error("stream contains file that should have been ignored")
		}
		if hdr.Name == "source/hello.txt" {
			helloTxtContent, err = io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if helloTxtContent == nil {
		t.Error("the hello.txt file is missing in the stream")
	} else {
		if string(helloTxtContent) != "Hello World!\n" {
			t.Error("the hello.txt file has incorrect content")
		}
	}
}
