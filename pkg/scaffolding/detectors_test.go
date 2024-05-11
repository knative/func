package scaffolding

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "knative.dev/func/pkg/testing"
)

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
		Name string    // Name of the test
		Sig  Signature // Signature Expected
		Err  error     // Error Expected
		Inv  string    // invocation hint; "http" (default) or "cloudevent"
		Src  string    // Source code to check
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
			Name: "Instanced Cloudevents",
			Sig:  InstancedCloudevents,
			Err:  nil,
			Inv:  "cloudevent", // Invoke is the only place Cloudevents is singular
			Src: `
package f
func New() { }
	`},
		{
			Name: "Static Cloudevents",
			Sig:  StaticCloudevents,
			Err:  nil,
			Inv:  "cloudevent",
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
		{
			Name: "Instanced with Handler",
			Sig:  InstancedHTTP,
			Err:  nil,
			Src: `
package f

type F struct{}

func New() *F { return &F{} }

func (f *MyFunction) Handle() {}
	`},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {

			root, cleanup := Mktemp(t)
			defer cleanup()

			if err := os.WriteFile(filepath.Join(root, "function.go"), []byte(test.Src), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			s, err := detectSignature(root, "go", test.Inv)
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
