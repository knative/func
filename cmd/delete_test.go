package cmd

import (
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func"
	"knative.dev/func/mock"
)

// TestDelete_Namespace ensures that the namespace provided to the client
// for use when deleting a function is set
// 1. The flag /env variable if provided
// 2. The namespace of the function at path if provided
// 3. The user's current active namespace
func TestDelete_Namespace(t *testing.T) {
	root := fromTempDirectory(t)

	// Ensure that the default is "default" when no context can be identified
	t.Setenv("KUBECONFIG", filepath.Join(cwd(), "nonexistent"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	cmd := NewDeleteCmd(func(cc ClientConfig, options ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "default" {
			t.Fatalf("expected 'default', got '%v'", cc.Namespace)
		}
		return fn.New(), func() {}
	})
	cmd.SetArgs([]string{"somefunc"}) // delete by name such that no f need be created
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Root:    root,
		Runtime: "go",
		Deploy: fn.DeploySpec{
			Namespace: "deployed",
		},
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}
	cmd = NewDeleteCmd(func(cc ClientConfig, options ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "deployed" {
			t.Fatalf("expected 'deployed', got '%v'", cc.Namespace)
		}
		return fn.New(), func() {}
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure an explicit namespace is plumbed through
	cmd = NewDeleteCmd(func(cc ClientConfig, options ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "ns" {
			t.Fatalf("expected 'ns', got '%v'", cc.Namespace)
		}
		return fn.New(), func() {}
	})
	cmd.SetArgs([]string{"--namespace", "ns"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

}

// TestDelete_ByName ensures that running delete specifying the name of the
// function explicitly as an argument invokes the remover appropriately.
func TestDelete_ByName(t *testing.T) {
	var (
		testname = "testname"        // explicit name for the function
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
	cmd := NewDeleteCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithRemover(remover))
	}))
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
	_ = fromTempDirectory(t)

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
	if err := os.WriteFile("func.yaml", []byte(funcYaml), 0600); err != nil {
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
	cmd := NewDeleteCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithRemover(remover))
	}))
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

// TestDelete_NameAndPathExclusivity ensures that providing both a name and a
// path generates an error.
// Providing the --path (-p) flag indicates the name of the function to delete
// is to be taken from the function at the given path.  Providing the name as
// an argument as well is therefore redundant and an error.
func TestDelete_NameAndPathExclusivity(t *testing.T) {

	// A mock remover which will be sampled to ensure it is not invoked.
	remover := mock.NewRemover()

	// Command with a Client constructor using the mock remover.
	cmd := NewDeleteCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithRemover(remover))
	}))

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
