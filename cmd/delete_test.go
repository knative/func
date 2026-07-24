package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestDelete_Default ensures that the deployed function is deleted correctly
// with default options and the default situation: running "delete" from
// within the same directory of the function which is to be deleted.
func TestDelete_Default(t *testing.T) {
	var (
		err       error
		root      = FromTempDirectory(t)
		name      = "myfunc"
		namespace = "testns"
		remover   = mock.NewRemover()
		ctx       = t.Context()
	)

	// Remover which confirms the name and namespace received are those
	// originally requested via the CLI flags.
	remover.RemoveFn = func(n, ns string) error {
		if n != name {
			t.Errorf("expected name '%v', got '%v'", name, n)
		}
		if ns != namespace {
			t.Errorf("expected namespace '%v', got '%v'", namespace, ns)
		}
		return nil
	}

	// A function which will be created in the requested namespace
	f := fn.Function{
		Runtime:   "go",
		Name:      name,
		Namespace: namespace,
		Root:      root,
		Registry:  TestRegistry,
	}

	if _, f, err = fn.New().New(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	cmd.SetArgs([]string{})
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
		root          = FromTempDirectory(t)
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
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	cmd.SetArgs([]string{testname}) // run: func delete <name>
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Fail if remover's .Remove not invoked at all
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}
}

// TestDelete_Namespace ensures that remover is invoked when --namespace flag is
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

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	cmd.SetArgs([]string{testname, "--namespace", namespace})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}
}

// TestDelete_NamespaceFlagPriority ensures that even though there is
// a deployed function the namespace flag takes precedence and essentially
// ignores the function on disk
func TestDelete_NamespaceFlagPriority(t *testing.T) {
	var (
		root       = FromTempDirectory(t)
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
	_, _, err = client.New(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
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
	_ = FromTempDirectory(t)

	cmd := NewDeleteCmd(NewTestClient())
	cmd.SetArgs([]string{"--namespace=myns"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("invoking Delete with namespace BUT without name provided anywhere")
	}
}

// TestDelete_ByProject ensures that running delete with a valid project as its
// context invokes remove and with the correct name (reads name from func.yaml)
func TestDelete_ByProject(t *testing.T) {
	_ = FromTempDirectory(t)

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
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
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
		root      = FromTempDirectory(t)
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
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))

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
	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))

	// Capture command output for inspection
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute the command simulating the invalid argument combination of both
	// a path and an explicit name.
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error on conflicting flags not received")
	} else if !errors.Is(err, ErrNameAndPathConflict) {
		t.Fatalf("expected ErrNameAndPathConflict, got %v", err)
	}

	// Also fail if remover's .Remove is invoked.
	if remover.RemoveInvoked {
		t.Fatal("fn.Remover invoked despite invalid combination and an error")
	}
}

// TestDelete_NoFunctionAtPath ensures delete returns a not-initialized error when no function exists at path
func TestDelete_NoFunctionAtPath(t *testing.T) {
	_ = FromTempDirectory(t)

	cmd := NewDeleteCmd(NewTestClient())
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when deleting without a function in path")
	}
	if !strings.Contains(err.Error(), "No function found in provided path") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestDelete_ByProjectClearsDeployedMarker ensures that a successful
// path-based delete persists a cleared function on disk so we have the
// function locally and on-cluster synced.
func TestDelete_ByProjectClearsDeployedMarker(t *testing.T) {
	root := FromTempDirectory(t)
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Deployer: keda.KedaDeployerName, // intent - how to deploy
		Deploy:   fn.DeploySpec{Namespace: "myns", Deployer: keda.KedaDeployerName},
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	remover := mock.NewRemover()
	remover.RemoveFn = func(_, _ string) error { return nil }

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}

	loaded, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Deploy.Namespace != "" {
		t.Fatalf("expected Deploy.Namespace cleared after a successful undeploy, got %q", loaded.Deploy.Namespace)
	}
	if loaded.Deploy.Deployer != "" {
		t.Fatalf("expected Deploy.Deployer cleared after a successful undeploy, got %q", loaded.Deploy.Deployer)
	}
	if loaded.Deployer != keda.KedaDeployerName {
		t.Fatalf("expected the intended Deployer preserved as a remembered choice, got %q", loaded.Deployer)
	}
}

// TestDelete_ByNameLeavesLocalFunctionUntouched ensures delete-by-name does NOT
// modify the local function's deployed state, even when a local function
// exists in the working directory. This is the distinction we make, its the
// caller's responsibility (of client.Remove) to deal with this.
func TestDelete_ByNameLeavesLocalFunctionUntouched(t *testing.T) {
	root := FromTempDirectory(t)
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Name:     "localfn",
		Deploy:   fn.DeploySpec{Namespace: "myns", Deployer: keda.KedaDeployerName},
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	remover := mock.NewRemover()
	remover.RemoveFn = func(n, _ string) error {
		if n != "otherfn" {
			t.Fatalf("expected delete-by-name target %q, got %q", "otherfn", n)
		}
		return nil
	}

	cmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	cmd.SetArgs([]string{"otherfn"}) // explicit name, distinct from the local function
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !remover.RemoveInvoked {
		t.Fatal("fn.Remover not invoked")
	}

	loaded, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Deploy.Namespace != "myns" {
		t.Fatalf("expected delete-by-name to leave the local function untouched, Deploy.Namespace changed to %q", loaded.Deploy.Namespace)
	}
}

// TestDelete_ByProjectPreservesDeployerForRedeploy ensures that redeploying
// after removal keeps the INTENT deployer intact and functional.
func TestDelete_ByProjectPreservesDeployerForRedeploy(t *testing.T) {
	root := FromTempDirectory(t)
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}

	// Deploy with keda
	deployCmd := NewDeployCmd(NewTestClient(fn.WithDeployer(mock.NewDeployer())))
	deployCmd.SetArgs([]string{"--deployer", keda.KedaDeployerName, "--namespace", "myns"})
	if err := deployCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Undeploy by project
	remover := mock.NewRemover()
	remover.RemoveFn = func(_, _ string) error { return nil }
	deleteCmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	deleteCmd.SetArgs([]string{})
	if err := deleteCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Flag-less redeploy: must reuse the persisted keda deployer.
	deployer := mock.NewDeployer()
	redeployCmd := NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
	redeployCmd.SetArgs([]string{})
	if err := redeployCmd.Execute(); err != nil {
		t.Fatalf("expected a flag-less redeploy after delete to succeed, got: %v", err)
	}
	if !deployer.DeployInvoked {
		t.Fatal("expected the deployer to be invoked on the redeploy")
	}

	loaded, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Deploy.Deployer != keda.KedaDeployerName {
		t.Fatalf("expected the flag-less redeploy to reuse the persisted %q deployer, got %q", keda.KedaDeployerName, loaded.Deploy.Deployer)
	}
}

// TestDelete_ByProjectThenRedeployWithDifferentDeployerNotBlocked ensures that
// undeploy removes "deployed status" effectively unblocking the subsequent
// re-deployment with different deployer
func TestDelete_ByProjectThenRedeployWithDifferentDeployerNotBlocked(t *testing.T) {
	root := FromTempDirectory(t)
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Registry: TestRegistry,
		Deploy:   fn.DeploySpec{Namespace: "myns", Deployer: keda.KedaDeployerName},
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	remover := mock.NewRemover()
	remover.RemoveFn = func(_, _ string) error { return nil }
	deleteCmd := NewDeleteCmd(NewTestClient(fn.WithRemovers(remover)))
	deleteCmd.SetArgs([]string{})
	if err := deleteCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	deployer := mock.NewDeployer()
	deployCmd := NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
	deployCmd.SetArgs([]string{"--deployer", deployers.Kubernetes})
	if err := deployCmd.Execute(); err != nil {
		t.Fatalf("expected the redeploy with a different deployer to succeed after undeploy, got: %v", err)
	}
	if !deployer.DeployInvoked {
		t.Fatal("expected the deployer to be invoked - the guard must not fire on an undeployed function")
	}
}
