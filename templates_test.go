// +build !integration

package function

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const TestRuntime = "test"

// TestWriteEmbedded ensures that embedded templates are copied.
func TestWriteEmbedded(t *testing.T) {
	// create test directory
	root := "testdata/testWriteEmbedded"
	defer using(t, root)()

	// write out a template
	w := templateWriter{}
	err := w.Write(TestRuntime, "tpla", root)
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "rtAtplA.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestWriteCustom ensures that a template from a filesystem source (ie. custom
// provider on disk) can be specified as the source for a template.
func TestWriteCustom(t *testing.T) {
	// Create test directory
	root := "testdata/testWriteFilesystem"
	defer using(t, root)()

	// Writer which includes custom repositories
	w := templateWriter{repositories: "testdata/repositories"}
	err := w.Write(TestRuntime, "customProvider/tpla", root)
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "customtpl.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestWriteDefault ensures that the default template is used when not specified.
func TestWriteDefault(t *testing.T) {
	// create test directory
	root := "testdata/testWriteDefault"
	defer using(t, root)()

	// write out a template
	w := templateWriter{}
	err := w.Write(TestRuntime, "", root)
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "rtAtplDefault.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestWriteModeDefault ensires that files written to disk are set to writible by the owner by default.  Since the embedded filesystem is expressly read-only, copying files to disk results in likewise read-only files.  Templates are expected to be mutable, so by default set mode to 0644.
func TestWriteModeDefault(t *testing.T) {
	root := "testdata/testWriteModeDefault"
	defer using(t, root)()

	w := templateWriter{}
	err := w.Write(TestRuntime, "tpla", root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file mode was defaulted
	f, err := os.Stat(filepath.Join(root, "rtAtplA.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode() != os.FileMode(0644) {
		t.Fatalf("The custom file's mode should be 0644 but was %#o", f.Mode())
	}

}

// TestWriteCustomMode ensures that templates written from custom templates
// retain their mode.  Note that the embed system is expressly for read-only
// filesystems, so the mode is always read-only
// https://golang.org/src/embed/embed.go?s=5559:6576#L235
func TestWriteModeCustom(t *testing.T) {
	if runtime.GOOS == "windows" {
		return // not applicable
	}

	// test directories
	var err error
	root := "testdata/testWriteModeCustom"
	defer using(t, root)()

	// Write executable from custom repo
	w := templateWriter{repositories: "testdata/repositories"}
	err = w.Write(TestRuntime, "customProvider/tplb", root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify custom file mode was preserved.
	customFile, err := os.Stat(filepath.Join(root, "custom-executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if customFile.Mode() != os.FileMode(0755) {
		t.Fatalf("The custom file's mode should be 0755 but was %v", customFile.Mode())
	}
}

// Helpers
// -------

// using the given directory (creating it) returns a closure which removes the
// directory, intended to be run in a defer statement.
func using(t *testing.T, root string) func() {
	t.Helper()
	mkdir(t, root)
	return func() {
		rm(t, root)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0744); err != nil {
		t.Fatal(err)
	}
}

func rm(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}
