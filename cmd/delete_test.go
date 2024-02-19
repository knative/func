package cmd

import (
	"context"
	"os"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestDelete_Default ensures that the deployed function is deleted correctly
// with default options
func TestDelete_Default(t *testing.T) {
	var (
		root      = fromTempDirectory(t)
		namespace = "myns"
		remover   = mock.NewRemover()
		err       error
	)

	remover.RemoveFn = func(_, ns string) error {
		if ns != namespace {
			t.Fatalf("expected delete namespace '%v', got '%v'", namespace, ns)
		}
		return nil
	}

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Name:     "testname",
		Deploy: fn.DeploySpec{
			Namespace: namespace, //simulate deployed Function
		},
	}

	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))
	cmd.SetArgs([]string{}) //dont give any arguments to 'func delete' -- default
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Fail if remover's .Remove not invoked at all
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_ByName ensures that running delete specifying the name of the
// function explicitly as an argument invokes the remover appropriately.
func TestDelete_ByName(t *testing.T) {
	var (
		root          = fromTempDirectory(t)
		testname      = "testname"        // explicit name for the function
		testnamespace = "testnamespace"   // explicit namespace for the function
		remover       = mock.NewRemover() // with a mock remover
		err           error
	)

	// Remover fails the test if it receives the incorrect name
	remover.RemoveFn = func(n, _ string) error {
		if n != testname {
			t.Fatalf("expected delete name %v, got %v", testname, n)
		}
		return nil
	}

	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Name:     "testname",
	}

	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	// simulate deployed function in namespace for the client Remover
	f.Deploy.Namespace = testnamespace

	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	// Create a command with a client constructor fn that instantiates a client
	// with a mocked remover.
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))
	cmd.SetArgs([]string{testname}) // run: func delete <name>

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Fail if remover's .Remove not invoked at all
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_Namespace ensures that remover is envoked when --namespace flag is
// given --> func delete myfunc --namespace myns
func TestDelete_Namespace(t *testing.T) {
	var (
		namespace = "myns"
		remover   = mock.NewRemover()
		testname  = "testname"
	)

	remover.RemoveFn = func(_, ns string) error {
		if ns != namespace {
			t.Fatalf("expected delete namespace '%v', got '%v'", namespace, ns)
		}
		return nil
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))
	cmd.SetArgs([]string{testname, "--namespace", namespace})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}
}

// TestDelete_NamespaceFlagPriority ensures that even thought there is
// a deployed function the namespace flag takes precedence and essentially
// ignores the the function on disk
func TestDelete_NamespaceFlagPriority(t *testing.T) {
	var (
		root       = fromTempDirectory(t)
		namespace  = "myns"
		namespace2 = "myns2"
		remover    = mock.NewRemover()
		testname   = "testname"
		err        error
	)

	remover.RemoveFn = func(_, ns string) error {
		if ns != namespace2 {
			t.Fatalf("expected delete namespace '%v', got '%v'", namespace2, ns)
		}
		return nil
	}

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Name:      testname,
		Root:      root,
		Runtime:   "go",
		Registry:  TestRegistry,
		Namespace: namespace,
	}
	client := fn.New()
	_, _, err = client.New(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))
	cmd.SetArgs([]string{testname, "--namespace", namespace2})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}
}

// TestDelete_NamespaceWithoutNameFails ensures that providing wrong argument
// combination fails nice and fast (no name of the Function)
func TestDelete_NamespaceWithoutNameFails(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewDeleteCmd(NewTestClient())
	cmd.SetArgs([]string{"--namespace=myns"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("invoking Delete with namespace BUT without name provided anywhere")
	}
}

// TestDelete_ByProject ensures that running delete with a valid project as its
// context invokes remove and with the correct name (reads name from func.yaml)
func TestDelete_ByProject(t *testing.T) {
	_ = fromTempDirectory(t)

	// Write a func.yaml config which specifies a name
	funcYaml := `name: bar
namespace: "func"
runtime: go
image: ""
builder: quay.io/boson/faas-go-builder
builders:
  default: quay.io/boson/faas-go-builder
envs: []
annotations: {}
labels: []
created: 2021-01-01T00:00:00+00:00
`
	if err := os.WriteFile("func.yaml", []byte(funcYaml), 0600); err != nil {
		t.Fatal(err)
	}

	// A mock remover which fails if the name from the func.yaml is not received.
	remover := mock.NewRemover()
	remover.RemoveFn = func(n, _ string) error {
		if n != "bar" {
			t.Fatalf("expected name 'bar', got '%v'", n)
		}
		return nil
	}

	// Command with a Client constructor that returns  client with the
	// mocked remover.
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))
	cmd.SetArgs([]string{}) // Do not use test command args

	// Execute the command simulating no arguments.
	err := cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// Also fail if remover's .Remove is not invoked
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_ByPath ensures that providing only path deletes the Function
// successfully
func TestDelete_ByPath(t *testing.T) {
	var (

		// A mock remover which will be sampled to ensure it is not invoked.
		remover   = mock.NewRemover()
		root      = fromTempDirectory(t)
		err       error
		namespace = "func"
	)

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Deploy:   fn.DeploySpec{Namespace: namespace},
	}

	// Initialize a function in temp dir
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	// Command with a Client constructor using the mock remover.
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))

	// Execute the command only with the path argument
	cmd.SetArgs([]string{"-p", root})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("failed with: %v", err)
	}

	// Also fail if remover's .Remove is not invoked.
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked despite valid argument")
	}
}

// TestDelete_NameAndPathExclusivity ensures that providing both a name and a
// path generates an error.
// Providing the --path (-p) flag indicates the name of the function to delete
// is to be taken from the function at the given path.  Providing the name as
// an argument as well is therefore redundant and an error.
func TestDelete_NameAndPathExclusivity(t *testing.T) {

	// A mock remover which will be sampled to ensure it is not invoked.
	remover := mock.NewRemover()

	// Command with a Client constructor using the mock remover.
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemover(remover)))

	// Execute the command simulating the invalid argument combination of both
	// a path and an explicit name.
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	err := cmd.Execute()
	if err == nil {
		// TODO should really either parse the output or use typed errors to ensure it's
		// failing for the expected reason.
		t.Fatalf("expected error on conflicting flags not received")
	}

	// Also fail if remover's .Remove is invoked.
	if remover.RemoveInvoked {
		t.Fatal("fn.Remover invoked despite invalid combination and an error")
	}
}
