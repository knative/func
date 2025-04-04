//go:build !integration
// +build !integration

package scaffolding

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"knative.dev/func/pkg/filesystem"

	. "knative.dev/func/pkg/testing"
)

// TestWrite_RuntimeErrors ensures that known runtimes which are not
// yet implemented return a "not yet available" message, and unrecognized
// runtimes state as much.
func TestWrite_RuntimeErrors(t *testing.T) {
	tests := []struct {
		Runtime  string
		Expected any
	}{
		{"go", nil},
		{"python", nil},
		{"rust", &ErrDetectorNotImplemented{}},
		{"node", &ErrDetectorNotImplemented{}},
		{"typescript", &ErrDetectorNotImplemented{}},
		{"quarkus", &ErrDetectorNotImplemented{}},
		{"java", &ErrDetectorNotImplemented{}},
		{"other", &ErrRuntimeNotRecognized{}},
	}
	for _, test := range tests {
		t.Run(test.Runtime, func(t *testing.T) {
			// Since runtime validation during signature detection is the very first
			// thing that occurs, we can elide most of the setup and pass zero
			// values for source directory, output directory and invocation.
			// This may need to be expanded in the event the Write function is
			// expanded to have more preconditions.
			err := Write("", "", test.Runtime, "", nil)
			if test.Expected != nil && err == nil {
				t.Fatalf("expected runtime %v to yield a detection error", test.Runtime)
			}
			if test.Expected != nil && !errors.As(err, test.Expected) {
				t.Fatalf("did not receive expected error type for %v runtime.", test.Runtime)
			}
			t.Logf("ok: %v", err)
		})
	}
}

// TestWrite ensures that the Write method writes Scaffolding to the given
// destination.  This is a fairly shallow test.  See the Scaffolding and
// Detector tests for more depth.
func TestWrite(t *testing.T) {
	// The filesystem containing scaffolding is expected to conform to the
	// structure:
	// /[language]/scaffolding/["instanced"|"static"]-[invocation]
	// ex:
	// "./go/scaffolding/instanced-http/main.go"
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fs := filesystem.NewOsFilesystem(filepath.Join(cwd, "testdata", "testwrite"))

	root, done := Mktemp(t)
	defer done()

	// Write out a test implementation that will result in the InstancedHTTP
	// signature being detected.
	impl := `
package f

type F struct{}

func New() *F { return nil }
`
	err = os.WriteFile(filepath.Join(root, "f.go"), []byte(impl), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module foo"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// The output destination for the scaffolding
	out := filepath.Join(root, "out")

	// Write Scaffolding to
	err = Write(
		out,  // output directory
		root, // source code location
		"go", // Runtime
		"",   // optional invocation hint (http is the default)
		fs)   // The filesystem from which the scaffolding should be pulled
	if err != nil {
		t.Fatal(err)
	}

	// Assert there exists a main.go (from the testdata scaffolding filesystem).
	if _, err = os.Stat(filepath.Join(root, "out", "main.go")); err != nil {
		t.Fatal(err)
	}

	// Assert there exists a symbolic link to the source code
	root, err = filepath.EvalSymlinks(root) // dereference any current symlinks
	if err != nil {
		t.Fatal(err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(out, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if target != root {
		t.Fatalf("scaffolding symlink should be:\n%v\n But got target:\n%v", root, target)
	}

}

// TestWrite_ScaffoldingNotFound ensures that a typed error is returned
// when scaffolding is not found.
func TestWrite_ScaffoldingNotFound(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fs := filesystem.NewOsFilesystem(filepath.Join(cwd, "testdata", "testnotfound"))

	root, done := Mktemp(t)
	defer done()

	impl := `
package f

type F struct{}

func New() *F { return nil }
`
	err = os.WriteFile(filepath.Join(root, "f.go"), []byte(impl), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(root, "out")

	err = Write(out, root, "go", "", fs)
	if err == nil {
		t.Fatal("did not receive expected error")
	}
	if !errors.Is(err, ErrScaffoldingNotFound) {
		t.Fatalf("error received was not ErrScaffoldingNotFound. %v", err)
	}
}

// TestNewScaffoldingError ensures that a scaffolding error wraps its
// underlying error such that callers can use errors.Is/As.
func TestNewScaffoldingError(t *testing.T) {

	// exampleError that would come from something scaffolding employs to
	// accomplish a task
	var ExampleError = errors.New("example error")

	err := ScaffoldingError{"some ExampleError", ExampleError}

	if !errors.Is(err, ExampleError) {
		t.Fatalf("type ScaffoldingError does not wrap errors.")
	}
	t.Logf("ok: %v", err)

}
