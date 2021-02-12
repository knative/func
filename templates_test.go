// +build !integration

package function

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
	function := Function{Root: path, Runtime: "quarkus", Trigger: "events"}
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
		path      = "testdata/example.com/www"
		template  = "boson-experimental/http"
		templates = "testdata/templates"
	)
	err := os.MkdirAll(path, 0744)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)

	client := New(WithTemplates(templates))
	function := Function{Root: path, Runtime: "quarkus", Trigger: template}
	if err := client.Create(function); err != nil {
		t.Fatal(err)
	}

	// Assert mode is preserved
	source, err := os.Stat(filepath.Join("testdata/templates/boson-experimental/quarkus/http/mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	dest, err := os.Stat(filepath.Join(path, "mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if dest.Mode() != source.Mode() {
			t.Fatalf("The dest mode should be %v but was %v", source.Mode(), dest.Mode())
		}
	}
}
