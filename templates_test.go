// +build !integration

package function

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRuntime consists of a specially designed templates directory
// used exclusively for embedded template write tests.
const TestRuntime = "test"

// TestTemplatesEmbeddedFileMode ensures that files from the embedded templates are
// written with the same mode from whence they came
func TestTemplatesEmbeddedFileMode(t *testing.T) {
	var path = "testdata/example.com/www"
	err := os.MkdirAll(path, 0744)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)

	client := New()
	function := Function{Root: path, Runtime: "quarkus", Template: "events"}
	if err := client.Create(function); err != nil {
		t.Fatal(err)
	}

	// The file mode of the embedded mvnw should be -rwxr-xr-x
	// See source file at: templates/quarkus/events/mvnw
	// Assert mode is preserved
	sourceMode := os.FileMode(0755)
	dest, err := os.Stat(filepath.Join(path, "mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if dest.Mode() != sourceMode {
			t.Fatalf("The dest mode should be %v but was %v", sourceMode, dest.Mode())
		}
	}
}

// TestTemplatesExtensibleFileMode ensures that files from a file-system
// derived template is written with mode retained.
func TestTemplatesExtensibleFileMode(t *testing.T) {
	var (
		path         = "testdata/example.com/www"
		template     = "customProvider/tplb"
		repositories = "testdata/repositories"
	)
	err := os.MkdirAll(path, 0744)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)

	client := New(WithRepositories(repositories))
	function := Function{Root: path, Runtime: TestRuntime, Template: template}
	if err := client.Create(function); err != nil {
		t.Fatal(err)
	}

	// Assert mode is preserved
	source, err := os.Stat(filepath.Join("testdata/repositories/customProvider/test/tplb/executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	dest, err := os.Stat(filepath.Join(path, "executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if dest.Mode() != source.Mode() {
			t.Fatalf("The dest mode should be %v but was %v", source.Mode(), dest.Mode())
		}
	}
}

// TestWriteInvalid ensures that specifying unrecgoznized runtimes or
// templates cause the related error.
func TestWriteInvalid(t *testing.T) {
	// create test directory
	root := "testdata/testWriteInvalid"
	err := os.MkdirAll(root, 0744)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	w := templateWriter{}

	// Test for error writing an invalid runtime
	// (the http template
	err = w.Write("invalid", DefaultTemplate, root)
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("Expected ErrRuntimeNotFound, got %v", err)
	}

	// Test for error writing an invalid template
	err = w.Write(TestRuntime, "invalid", root)
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("Expected ErrTemplateNotFound, got %v", err)
	}
}
