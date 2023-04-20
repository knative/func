package functions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "knative.dev/func/pkg/testing"
)

// TestSignature_Map ensures via spot-checking that the mappings for the
// different method signature constants are correctly associated to their
// string representation, the boolean indicator of instanced, and the
// invocation hint as defined on the function; and this association is
// traversable via the `signature` method.
func TestSignature_Map(t *testing.T) {
	instanced := false
	invocation := "http"
	expectedName := "static-http"
	expectedSig := StaticHTTP

	s := signature(instanced, invocation)
	if s != expectedSig {
		t.Fatal("signature flags incorrectly mapped")
	}
	if s.String() != expectedName {
		t.Fatalf("signature string representation incorrectly mapped.  Expected %q got %q", expectedName, s)
	}

	// ensure that the default for invocation is http
	if signature(true, "") != InstancedHTTP {
		t.Fatalf("expected %v, got %v", InstancedHTTP, signature(true, ""))
	}
}

// TestDetector_Go ensures that the go language detector will correctly
// identify the signature to expect of a function's source.
func TestDetector_Go(t *testing.T) {
	// NOTE:
	// The detector need only check the function's name; not the entire signature
	// because invocation hint (http vs cloudevent) is available in the
	// function's metadata, and the detector needs to be as simple as possible
	// while fulfilling its purpose of detecting which signature is _expected_
	// of the source code, not whether or not it actually does:  that's the job
	// of the compiler later.  This detection is used to determine which
	// scaffolding code needs to be written to get the user to a proper
	// complile attempt.
	tests := []struct {
		Name string                  // Name of the test
		Sig  Signature               // Signature Expected
		Err  error                   // Error Expected
		Src  string                  // Source code to check
		Cfg  func(Function) Function // Configure the default function for the test.
	}{
		{
			Name: "Instanced HTTP",
			Sig:  InstancedHTTP,
			Err:  nil,
			Src: `
package f

func New() { }
	`},
		{
			Name: "Static HTTP",
			Sig:  StaticHTTP,
			Err:  nil,
			Src: `
package f

func Handle() { }
	`},
		{
			Name: "Instanced Cloudevent",
			Sig:  InstancedCloudevent,
			Err:  nil,
			Cfg: func(f Function) Function {
				f.Invoke = "cloudevent" // see NOTE above
				return f
			},
			Src: `
package f
func New() { }
	`},
		{
			Name: "Static Cloudevent",
			Sig:  StaticCloudevent,
			Err:  nil,
			Cfg: func(f Function) Function {
				f.Invoke = "cloudevent" // see NOTE above
				return f
			},
			Src: `
package f
func Handle() { }
	`},
		{
			Name: "Static and Instanced - error",
			Sig:  UnknownSignature,
			Err:  errors.New("error expected"), // TODO: typed error and err.Is/As
			Src: `
package f
func Handle() { }
func New() { }
	`},
		{
			Name: "No Signatures Found - error",
			Sig:  UnknownSignature,
			Err:  errors.New("error expected"), // TODO: typed error and err.Is/As
			Src: `
package f
// Intentionally Blank
	`},
		{
			Name: "Comments Ignored",
			Sig:  StaticHTTP,
			Err:  nil,
			Src: `
package f
/*
This comment block would cause the function to be detected as instanced
without the use of the language parser.

func New()

*/
func Handle() { }
	`},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {

			root, cleanup := Mktemp(t)
			defer cleanup()

			f := Function{Runtime: "go", Root: root}
			if test.Cfg != nil {
				f = test.Cfg(f)
			}

			f, err := New().Init(f)
			if err != nil {
				t.Fatal(err)
			}

			// NOTE: if/when the default filename changes from handle.go to
			// function.go, this will also have to change
			os.WriteFile(filepath.Join(root, "handle.go"), []byte(test.Src), os.ModePerm)

			s, err := functionSignature(f)
			if err != nil && test.Err == nil {
				t.Fatalf("unexpected error. %v", err)
			}

			if test.Err != nil {
				if err == nil {
					t.Fatal("expected error not received")
				} else {
					t.Logf("received expected error: %v", err)
				}
			}

			if s != test.Sig {
				t.Fatalf("Expected signature '%v', got '%v'", test.Sig, s)
			}
		})
	}
}
