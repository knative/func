package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitialize ensures that on initialization of a the reference language
// (Go), the template is written.
func TestInitialize(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		context  = "http"
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	err := NewInitializer("").Initialize("go", context, path)
	if err != nil {
		t.Fatal(err)
	}

	// Test that the directory is not empty
	if _, err := os.Stat(filepath.Join(path, testFile)); os.IsNotExist(err) {
		t.Fatalf("Initialize did not result in '%v' being written to '%v'", testFile, path)
	}
}

// TestDefaultContext ensures that if no context is provided, files are still written.
func TestDefaultContext(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		context  = ""
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	err := NewInitializer("").Initialize("go", context, path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(path, testFile)); os.IsNotExist(err) {
		t.Fatalf("Initializing without providing a context did not result in '%v' being written to '%v'", testFile, path)
	}
}

// TestCustom ensures that a custom repository can be used as a template.
// Custom repository location is not defined herein but expected to be
// provided because, for example, a CLI may want to use XDG_CONFIG_HOME.
// Assuming a repository path $FAAS_TEMPLATES, a Go context named 'json'
// which is provided in the repository repository 'boson-experimental',
// would be expected to be in the location:
// $FAAS_TEMPLATES/boson-experimental/go/json
// See the CLI for full details, but a standard default location is
// $HOME/.config/templates/boson-experimental/go/json
func TestCustom(t *testing.T) {
	var (
		path     = "testdata/example.com/www"
		testFile = "handle.go"
		context  = "boson-experimental/json"
		// repos    = "testdata/templates"
	)
	os.MkdirAll(path, 0744)
	defer os.RemoveAll(path)

	// Unrecognized language/context should error
	err := NewInitializer("").Initialize("go", context, path)
	if err == nil {
		t.Fatal("An unrecognized language/context should generate an error")
	}

	// Recognized external (non-embedded) path should succeed
	err = NewInitializer("testdata/templates").Initialize("go", context, path)
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
