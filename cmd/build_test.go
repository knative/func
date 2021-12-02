package cmd

import (
	"io/ioutil"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

// TestBuildInvalidRegistry ensures that running build specifying the name of the
// registry explicitly as an argument invokes the registry validation code.
func TestBuildInvalidRegistry(t *testing.T) {
	var (
		args    = []string{"--registry", "foo/bar/foobar/boofar"} // provide an invalid registry name
		builder = mock.NewBuilder()                               // with a mock builder
	)

	// Run this test in a temporary directory
	defer fromTempDir(t)()
	// Write a func.yaml config which does not specify an image
	funcYaml := `name: testymctestface
namespace: ""
runtime: go
image: ""
imageDigest: ""
builder: quay.io/boson/faas-go-builder
builders:
  default: quay.io/boson/faas-go-builder
envs: []
annotations: {}
labels: []
created: 2021-01-01T00:00:00+00:00
`
	if err := ioutil.WriteFile("func.yaml", []byte(funcYaml), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a command with a client constructor fn that instantiates a client
	// with a the mocked builder.
	cmd := NewBuildCmd(func(_ buildConfig) (*fn.Client, error) {
		return fn.New(fn.WithBuilder(builder)), nil
	})

	// Execute the command
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Expected error")
	}
}
