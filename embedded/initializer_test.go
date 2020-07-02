package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitialize ensures that on initialization of a the reference runtime
// (Go), the template is written.
func TestInitialize(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		template = "http"
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	err := NewInitializer("").Initialize("go", template, path)
	if err != nil {
		t.Fatal(err)
	}

	// Test that the directory is not empty
	if _, err := os.Stat(filepath.Join(path, testFile)); os.IsNotExist(err) {
		t.Fatalf("Initialize did not result in '%v' being written to '%v'", testFile, path)
	}
}

// TestDefaultTemplate ensures that if no template is provided, files are still written.
func TestDefaultTemplate(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		template = ""
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	err := NewInitializer("").Initialize("go", template, path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(path, testFile)); os.IsNotExist(err) {
		t.Fatalf("Initializing without providing a template did not result in '%v' being written to '%v'", testFile, path)
	}
}

// TestCustom ensures that a custom repository can be used as a template.
// Custom repository location is not defined herein but expected to be
// provided because, for example, a CLI may want to use XDG_CONFIG_HOME.
// Assuming a repository path $FAAS_TEMPLATES, a Go template named 'json'
// which is provided in the repository repository 'boson-experimental',
// would be expected to be in the location:
// $FAAS_TEMPLATES/boson-experimental/go/json
// See the CLI for full details, but a standard default location is
// $HOME/.config/templates/boson-experimental/go/json
func TestCustom(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		template = "boson-experimental/json"
		// repos    = "testdata/templates"
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	// Unrecognized runtime/template should error
	err := NewInitializer("").Initialize("go", template, path)
	if err == nil {
		t.Fatal("An unrecognized runtime/template should generate an error")
	}

	// Recognized external (non-embedded) path should succeed
	err = NewInitializer("testdata/templates").Initialize("go", template, path)
	if err != nil {
		t.Fatal(err)
	}

	// The template should have been written to the given path.
	if _, err := os.Stat(filepath.Join(path, testFile)); os.IsNotExist(err) {
		t.Fatalf("Initializing a custom did not result in the expected '%v' being written to '%v'", testFile, path)
	} else if err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedFileMode ensures that files from the embedded templates are
// written with the same mode from whence they came
func TestEmbeddedFileMode(t *testing.T) {
	var path = "testdata/example.com/www"
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	// Initialize a java app from the embedded templates.
	if err := NewInitializer("").Initialize("java", "events", path); err != nil {
		t.Fatal(err)
	}

	// The file mode of the embedded mvnw should be -rwxr-xr-x
	// See source file at: templates/java/events/mvnw
	// Assert mode is preserved
	sourceMode := os.FileMode(0755)
	dest, err := os.Stat(filepath.Join(path, "mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	if dest.Mode() != sourceMode {
		t.Fatalf("The dest mode should be %v but was %v", sourceMode, dest.Mode())
	}
}

// TestCustomFileMode ensures that files from a file-system derived repository
// of templates are written with the same mode from whence they came
func TestFileMode(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		template = "boson-experimental/http"
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	// Initialize a java app from the custom repo in ./testdata
	if err := NewInitializer("testdata/templates").Initialize("java", template, path); err != nil {
		t.Fatal(err)
	}

	// Assert mode is preserved
	source, err := os.Stat(filepath.Join("testdata/templates/boson-experimental/java/http/mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	dest, err := os.Stat(filepath.Join(path, "mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	if dest.Mode() != source.Mode() {
		t.Fatalf("The dest mode should be %v but was %v", source.Mode(), dest.Mode())
	}
}
