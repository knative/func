package oci

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestNewPythonLibTarball_NormalizesModes ensures that the python lib layer
// tarball is written with portable permissions regardless of the on-disk modes
// of the build directory.
//
// Regression test: the build directory is created with 0774 which, under the
// default umask 022, becomes 0754 - stripping the traverse/read bit for
// group/other. When such a mode is copied verbatim into the image layer, the
// resulting container only works for the image's configured UID and fails with
// "[Errno 13] Permission denied" when run under an arbitrary UID (e.g. on
// OpenShift's restricted SCC). Directories and executables must be 0755 and
// regular files 0644 so any UID can traverse and read them.
func TestNewPythonLibTarball_NormalizesModes(t *testing.T) {
	root := t.TempDir()

	// Recreate the on-disk layout the python builder tars up:
	//   <root>/.func/build/service/main.py   (regular file, restrictive parent)
	//   <root>/.func/build/run.sh            (executable)
	buildDir := filepath.Join(root, fn.RunDataDir, fn.BuildDir)
	svcDir := filepath.Join(buildDir, "service")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Force the problematic non-portable modes.
	if err := os.WriteFile(filepath.Join(svcDir, "main.py"), []byte("print('hi')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// chmod parent dirs to 0754 (what 0774 & ~umask 022 yields).
	if err := os.Chmod(svcDir, 0o754); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(buildDir, 0o754); err != nil {
		t.Fatal(err)
	}

	job := buildJob{function: fn.Function{Root: root}}
	target := filepath.Join(root, "lib.tar.gz")
	if err := newPythonLibTarball(job, buildDir, target); err != nil {
		t.Fatal(err)
	}

	modes := map[string]int64{}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		modes[hdr.Name] = hdr.Mode & int64(fs.ModePerm)
	}

	assertMode := func(name string, want int64) {
		t.Helper()
		got, ok := modes[name]
		if !ok {
			t.Fatalf("entry %q not found in tarball; entries: %v", name, modes)
		}
		if got != want {
			t.Errorf("entry %q has mode %#o, want %#o", name, got, want)
		}
	}

	// Directories must be traversable by any UID.
	assertMode("/func/.func/build", 0o755)
	assertMode("/func/.func/build/service", 0o755)
	// Regular files must be readable by any UID.
	assertMode("/func/.func/build/service/main.py", 0o644)
	if runtime.GOOS != "windows" {
		// Executables keep the execute bit for any UID. Windows has no
		// executable bit, so the file is normalized as a regular file.
		assertMode("/func/.func/build/run.sh", 0o755)
	} else {
		assertMode("/func/.func/build/run.sh", 0o644)
	}
}
