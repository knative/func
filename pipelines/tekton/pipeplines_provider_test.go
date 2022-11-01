package tekton

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func"
)

func TestSourcesAsTarStream(t *testing.T) {
	root := filepath.Join("testData", "fn-src")

	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	rc := sourcesAsTarStream(fn.Function{Root: root})
	t.Cleanup(func() { _ = rc.Close() })

	var helloTxtContent []byte
	var symlinkFound bool
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
		if strings.HasPrefix(hdr.Name, "source/.git") {
			t.Errorf(".git files were included: %q", hdr.Name)
		}
		if hdr.Name == "source/hello.txt" {
			helloTxtContent, err = io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
		}
		if hdr.Name == "source/hello.txt.lnk" {
			symlinkFound = true
			if hdr.Linkname != "hello.txt" {
				t.Errorf("bad symlink target: %q", hdr.Linkname)
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
	if !symlinkFound {
		t.Error("symlink missing in the stream")
	}
}
