package cmd

import (
	"io/ioutil"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

// TestDelete_ByName ensures that running delete specifying the name of the
// Function explicitly as an argument invokes the remover appropriately.
func TestDelete_ByName(t *testing.T) {
	var (
		testname = "testname"        // explicit name for the Function
		remover  = mock.NewRemover() // with a mock remover
	)

	// Remover fails the test if it receives the incorrect name
	// an incorrect name.
	remover.RemoveFn = func(n string) error {
		if n != testname {
			t.Fatalf("expected delete name %v, got %v", testname, n)
		}
		return nil
	}

	// Create a command with a client constructor fn that instantiates a client
	// with a the mocked remover.
	cmd := NewDeleteCmd(
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
	)

	cmd.SetArgs([]string{testname})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Fail if remover's .Remove not invoked at all
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_ByProject ensures that running delete with a valid project as its
// context invokes remove and with the correct name (reads name from func.yaml)
func TestDelete_ByProject(t *testing.T) {
	// from within a new temporary directory
	defer fromTempDir(t)()

	// Write a func.yaml config which specifies a name
	funcYaml := `name: bar
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

	// A mock remover which fails if the name from the func.yaml is not received.
	remover := mock.NewRemover()
	remover.RemoveFn = func(n string) error {
		if n != "bar" {
			t.Fatalf("expected name 'bar', got '%v'", n)
		}
		return nil
	}

	// Command with a Client constructor that returns  client with the
	// mocked remover.
	cmd := NewDeleteCmd(
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
	)

	// Execute the command simulating no arguments.
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// Also fail if remover's .Remove is not invoked
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_NameAndPathExclusivity ensures that providing both a name and a
// path generates an error.
// Providing the --path (-p) flag indicates the name of the function to delete
// is to be taken from the Function at the given path.  Providing the name as
// an argument as well is therefore redundant and an error.
func TestDelete_NameAndPathExclusivity(t *testing.T) {

	// A mock remover which will be sampled to ensure it is not invoked.
	remover := mock.NewRemover()

	// Command with a Client constructor using the mock remover.
	cmd := NewDeleteCmd(
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
	)

	// Execute the command simulating the invalid argument combination of both
	// a path and an explicit name.
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	err := cmd.Execute()
	if err == nil {
		// TODO should really either parse the output or use typed errors to ensure it's
		// failing for the expected reason.
		t.Fatal(err)
	}

	// Also fail if remover's .Remove is invoked.
	if remover.RemoveInvoked {
		t.Fatal("fn.Remover invoked despite invalid combination and an error")
	}
}
