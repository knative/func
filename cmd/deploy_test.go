package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/mock"
)

// commandConstructor is used to share test implementations between commands
// which only differ in the command being tested (ex: build and deploy share
// a large overlap in tests because building is a subset of the deploy task)
type commandConstructor func(ClientFactory) *cobra.Command

// TestDeploy_BuilderPersists ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_BuilderPersists(t *testing.T) {
	testBuilderPersists(NewDeployCmd, t)
}

func testBuilderPersists(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}
	cmd := cmdFn(NewTestClient(fn.WithRegistry(TestRegistry)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var err error
	var f fn.Function

	// Assert the function has persisted a value of builder (has a default)
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder == "" {
		t.Fatal("value of builder not persisted using a flag default")
	}

	// Build the function, specifying a Builder
	viper.Reset()
	cmd.SetArgs([]string{"--builder=s2i"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Assert the function has persisted the value of builder
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder != builders.S2I {
		t.Fatalf("value of builder flag not persisted when provided. Expected '%v' got '%v'", builders.S2I, f.Build.Builder)
	}

	// Build the function again without specifying a Builder
	viper.Reset()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has retained its original value
	// (was not reset back to a flag default)
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder != builders.S2I {
		t.Fatal("value of builder updated when not provided")
	}

	// Build the function again using a different builder
	viper.Reset()
	cmd.SetArgs([]string{"--builder=pack"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has persisted the new value
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder != builders.Pack {
		t.Fatalf("value of builder flag not persisted on subsequent build. Expected '%v' got '%v'", builders.Pack, f.Build.Builder)
	}

	// Build the function, specifying a platform with "pack" Builder
	cmd.SetArgs([]string{"--platform", "linux"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error using --platform without s2i builder was not received")
	}

	// Set an invalid builder
	cmd.SetArgs([]string{"--builder", "invalid"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error using an invalid --builder not received")
	}
}

// TestDeploy_BuilderValidated ensures that the validation function correctly
// identifies valid and invalid builder short names.
func TestDeploy_BuilderValidated(t *testing.T) {
	testBuilderValidated(NewDeployCmd, t)
}

func testBuilderValidated(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewTestClient())

	cmd.SetArgs([]string{"--builder=invalid"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error string '%v', got '%v'", "expected", err.Error())
	}

}

// TestDeploy_ConfigApplied ensures that the deploy command applies config
// settings at each level (static, global, function, envs, flags)
func TestDeploy_ConfigApplied(t *testing.T) {
	testConfigApplied(NewDeployCmd, t)
}

func testConfigApplied(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	var (
		err      error
		home     = fmt.Sprintf("%s/testdata/TestX_ConfigApplied", cwd())
		root     = fromTempDirectory(t)
		f        = fn.Function{Runtime: "go", Root: root, Name: "f"}
		pusher   = mock.NewPusher()
		clientFn = NewTestClient(fn.WithPusher(pusher))
	)
	t.Setenv("XDG_CONFIG_HOME", home)

	if _, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	// Ensure the global config setting was loaded: Registry
	// global config in ./testdata/TestBuild_ConfigApplied sets registry
	if err = cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Registry != "registry.example.com/alice" {
		t.Fatalf("expected registry 'registry.example.com/alice' got '%v'", f.Registry)
	}

	// Ensure flags are evaluated
	cmd := cmdFn(clientFn)
	cmd.SetArgs([]string{"--builder-image", "example.com/builder/image:v1.2.3"})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.BuilderImages[f.Build.Builder] != "example.com/builder/image:v1.2.3" {
		t.Fatalf("expected builder image not set. Flags not evaluated? got %v", f.Build.BuilderImages[f.Build.Builder])
	}

	// Ensure function context loaded
	// Update registry on the function and ensure it takes precidence (overrides)
	// the global setting defined in home.
	f.Registry = "registry.example.com/charlie"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	if err := cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}

	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Image != "registry.example.com/charlie/f:latest" {
		t.Fatalf("expected image 'registry.example.com/charlie/f:latest' got '%v'", f.Build.Image)
	}

	// Ensure environment variables loaded: Push
	// Test environment variable evaluation using FUNC_PUSH
	t.Setenv("FUNC_PUSH", "true")
	if err := cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if !pusher.PushInvoked {
		t.Fatalf("push was not invoked when FUNC_PUSH=true")
	}
}

// TestDeploy_ConfigPrecedence ensures that the correct precidence for config
// are applied: static < global < function context < envs < flags
func TestDeploy_ConfigPrecedence(t *testing.T) {
	testConfigPrecedence(NewDeployCmd, t)
}

func testConfigPrecedence(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	var (
		err      error
		home     = fmt.Sprintf("%s/testdata/TestX_ConfigPrecedence", cwd())
		builder  = mock.NewBuilder()
		clientFn = NewTestClient(fn.WithBuilder(builder))
	)

	// Ensure static default applied via 'builder'
	// (a degenerate case, mostly just ensuring config values are not wiped to a
	// zero value struct, etc)
	root := fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", home) // sets registry.example.com/global
	f := fn.Function{Runtime: "go", Root: root, Name: "f"}
	if f, err = fn.New().Init(f); err != nil {
		t.Fatal(err)
	}
	if err := cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder != builders.Default {
		t.Fatalf("expected static default builder '%v', got '%v'", builders.Default, f.Build.Builder)
	}

	// Ensure Global Config applied via config in ./testdata
	root = fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", home) // sets registry.example.com/global
	f = fn.Function{Runtime: "go", Root: root, Name: "f"}
	f, err = fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err = cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Registry != "registry.example.com/global" { // from ./testdata
		t.Fatalf("expected registry 'example.com/global', got '%v'", f.Registry)
	}

	// Ensure Function context overrides global config
	// The stanza above ensures the global config is applied.  This stanza
	// ensures that, if set on the function, it will take precidence.
	root = fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", home) // sets registry=example.com/global
	f = fn.Function{Runtime: "go", Root: root, Name: "f",
		Registry: "example.com/function"}
	f, err = fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err = cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Registry != "example.com/function" {
		t.Fatalf("expected function's value for registry of 'example.com/function' to override global config setting of 'example.com/global', but got '%v'", f.Registry)
	}

	// Ensure Environment Variable overrides function context.
	root = fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", home) // sets registry.example.com/global
	t.Setenv("FUNC_REGISTRY", "example.com/env")
	f = fn.Function{Runtime: "go", Root: root, Name: "f",
		Registry: "example.com/function"}
	f, err = fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmdFn(clientFn).Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Registry != "example.com/env" {
		t.Fatalf("expected FUNC_REGISTRY=example.com/env to override function's value of 'example.com/function', but got '%v'", f.Registry)
	}

	// Ensure flags override environment variables.
	root = fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", home) // sets registry=example.com/global
	t.Setenv("FUNC_REGISTRY", "example.com/env")
	f = fn.Function{Runtime: "go", Root: root, Name: "f",
		Registry: "example.com/function"}
	f, err = fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	cmd := cmdFn(clientFn)
	cmd.SetArgs([]string{"--registry=example.com/flag"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Registry != "example.com/flag" {
		t.Fatalf("expected flag 'example.com/flag' to take precidence over env var, but got '%v'", f.Registry)
	}
}

// TestDeploy_Default ensures that running deploy on a valid default Function
// (only required options populated; all else default) completes successfully.
func TestDeploy_Default(t *testing.T) {
	testDefault(NewDeployCmd, t)
}

func testDefault(cmdFn commandConstructor, t *testing.T) {
	root := fromTempDirectory(t)

	// A Function with the minimum required values for deployment populated.
	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Execute using an instance of the command which uses a fully default
	// (noop filled) Client.  Execution should complete without error.
	cmd := cmdFn(NewTestClient())
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_Envs ensures that environment variable for the function, provided
// as arguments, are correctly evaluated.  This includes:
// - Multiple Envs are supported (flag can be provided multiple times)
// - Existing Envs on the function are retained
// - Flags provided with the minus '-' suffix are treated as a removal
func TestDeploy_Envs(t *testing.T) {
	var (
		root     = fromTempDirectory(t)
		ptr      = func(s string) *string { return &s } // TODO: remove pointers from Envs.
		f        fn.Function
		cmd      *cobra.Command
		err      error
		expected []fn.Env
	)

	f, err = fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Deploy the function with two environment variables specified.
	cmd = NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--env=ENV1=VAL1", "--env=ENV2=VAL2"})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Assert it contains the two environment variables
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	expected = []fn.Env{
		{Name: ptr("ENV1"), Value: ptr("VAL1")},
		{Name: ptr("ENV2"), Value: ptr("VAL2")},
	}
	if !reflect.DeepEqual(f.Run.Envs, fn.Envs(expected)) {
		t.Fatalf("Expected envs '%v', got '%v'", expected, f.Run.Envs)
	}

	// Deploy the function with an additinal environment variable.
	cmd = NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--env=ENV3=VAL3"})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Assert the original envs were retained and the new env was added.
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	expected = []fn.Env{
		{Name: ptr("ENV1"), Value: ptr("VAL1")},
		{Name: ptr("ENV2"), Value: ptr("VAL2")},
		{Name: ptr("ENV3"), Value: ptr("VAL3")},
	}
	if !reflect.DeepEqual(f.Run.Envs, fn.Envs(expected)) {
		t.Fatalf("Expected envs '%v', got '%v'", expected, f.Run.Envs)
	}

	// Deploy the function with a removal instruction
	cmd = NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--env=ENV1-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	expected = []fn.Env{
		{Name: ptr("ENV2"), Value: ptr("VAL2")},
		{Name: ptr("ENV3"), Value: ptr("VAL3")},
	}
	if !reflect.DeepEqual(f.Run.Envs, fn.Envs(expected)) {
		t.Fatalf("Expected envs '%v', got '%v'", expected, f.Run.Envs)
	}

	// Try removing the rest for good measure
	cmd = NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--env=ENV2-", "--env=ENV3-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if len(f.Run.Envs) != 0 {
		t.Fatalf("Expected no envs to remain, got '%v'", f.Run.Envs)
	}

	// TODO: create and test typed errors for ErrEnvNotExist etc.
}

// TestDeploy_FunctionContext ensures that the function contextually relevant
// to the current command is loaded and used for flag defaults by spot-checking
// the builder setting.
func TestDeploy_FunctionContext(t *testing.T) {
	testFunctionContext(NewDeployCmd, t)
}

func testFunctionContext(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Build the function explicitly setting the builder to !builders.Default
	cmd := cmdFn(NewTestClient())
	dflt := cmd.Flags().Lookup("builder").DefValue

	// The initial default value should be builders.Default (see global config)
	if dflt != builders.Default {
		t.Fatalf("expected flag default value '%v', got '%v'", builders.Default, dflt)
	}

	// Choose the value that is not the default
	// We must calculate this because downstream changes the default via patches.
	var builder string
	if builders.Default == builders.Pack {
		builder = builders.S2I
	} else {
		builder = builders.Pack
	}

	// Build with the other
	cmd.SetArgs([]string{"--builder", builder})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// The function should now have the builder set to the new builder
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Build.Builder != builder {
		t.Fatalf("expected function to have new builder '%v', got '%v'", builder, f.Build.Builder)
	}

	// The command default should now take into account the function when
	// determining the flag default
	cmd = cmdFn(NewTestClient())
	dflt = cmd.Flags().Lookup("builder").DefValue

	if dflt != builder {
		t.Fatalf("expected flag default to be function's current builder '%v', got '%v'", builder, dflt)
	}
}

// TestDeploy_GitArgsPersist ensures that the git flags, if provided, are
// persisted to the Function for subsequent deployments.
func TestDeploy_GitArgsPersist(t *testing.T) {
	root := fromTempDirectory(t)

	var (
		url    = "https://example.com/user/repo"
		branch = "main"
		dir    = "function"
	)

	// Create a new Function in the temp directory
	f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Deploy the Function specifying all of the git-related flags
	cmd := NewDeployCmd(NewTestClient(
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir, "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Load the Function and ensure the flags were stored.
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Build.Git.URL != url {
		t.Errorf("expected git URL '%v' got '%v'", url, f.Build.Git.URL)
	}
	if f.Build.Git.Revision != branch {
		t.Errorf("expected git branch '%v' got '%v'", branch, f.Build.Git.Revision)
	}
	if f.Build.Git.ContextDir != dir {
		t.Errorf("expected git dir '%v' got '%v'", dir, f.Build.Git.ContextDir)
	}
}

// TestDeploy_GitArgsUsed ensures that any git values provided as flags are used
// when invoking a remote deployment.
func TestDeploy_GitArgsUsed(t *testing.T) {
	root := fromTempDirectory(t)

	var (
		url    = "https://example.com/user/repo"
		branch = "main"
		dir    = "function"
	)
	// Create a new Function in the temp dir
	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// A Pipelines Provider which will validate the expected values were received
	pipeliner := mock.NewPipelinesProvider()
	pipeliner.RunFn = func(f fn.Function) (string, string, error) {
		if f.Build.Git.URL != url {
			t.Errorf("Pipeline Provider expected git URL '%v' got '%v'", url, f.Build.Git.URL)
		}
		if f.Build.Git.Revision != branch {
			t.Errorf("Pipeline Provider expected git branch '%v' got '%v'", branch, f.Build.Git.Revision)
		}
		if f.Build.Git.ContextDir != dir {
			t.Errorf("Pipeline Provider expected git dir '%v' got '%v'", url, f.Build.Git.ContextDir)
		}
		return url, "", nil
	}

	// Deploy the Function specifying all of the git-related flags and --remote
	// such that the mock pipelines provider is invoked.
	cmd := NewDeployCmd(NewTestClient(
		fn.WithPipelinesProvider(pipeliner),
		fn.WithRegistry(TestRegistry),
	))

	cmd.SetArgs([]string{"--remote=true", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_GitURLBranch ensures that a --git-url which specifies the branch
// in the URL is equivalent to providing --git-branch
func TestDeploy_GitURLBranch(t *testing.T) {
	root := fromTempDirectory(t)

	f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	var (
		url            = "https://example.com/user/repo#branch"
		expectedUrl    = "https://example.com/user/repo"
		expectedBranch = "branch"
	)
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Build.Git.URL != expectedUrl {
		t.Errorf("expected git URL '%v' got '%v'", expectedUrl, f.Build.Git.URL)
	}
	if f.Build.Git.Revision != expectedBranch {
		t.Errorf("expected git branch '%v' got '%v'", expectedBranch, f.Build.Git.Revision)
	}
}

// TestDeploy_ImageAndRegistry ensures that image is derived when --registry
// is provided without --image; that --image is used if provided; that when
// both are provided, they are both passed to the deployer; and that these
// values are persisted.
func TestDeploy_ImageAndRegistry(t *testing.T) {
	testImageAndRegistry(NewDeployCmd, t)
}

func testImageAndRegistry(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	var (
		builder  = mock.NewBuilder()
		deployer = mock.NewDeployer()
		cmd      = cmdFn(NewTestClient(fn.WithBuilder(builder), fn.WithDeployer(deployer), fn.WithRegistry(TestRegistry)))
	)

	// If only --registry is provided:
	// the resultant Function should have the registry populated and image
	// derived from the name.
	cmd.SetArgs([]string{"--registry=example.com/alice"})
	deployer.DeployFn = func(_ context.Context, f fn.Function) (res fn.DeploymentResult, err error) {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag not provided to deployer")
		}
		return
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If only --image is provided:
	// the deploy should not fail, and the resultant Function should have the
	// Image member set to what was explicitly provided via the --image flag
	// (not a derived name)
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	deployer.DeployFn = func(_ context.Context, f fn.Function) (res fn.DeploymentResult, err error) {
		if f.Image != "example.com/alice/myfunc" {
			t.Fatalf("deployer expected f.Image 'example.com/alice/myfunc', got '%v'", f.Image)
		}
		return
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If both --registry and --image are provided:
	// they should both be plumbed through such that downstream agents (deployer
	// in this case) see them set on the Function and can act accordingly.
	cmd.SetArgs([]string{"--registry=example.com/alice", "--image=example.com/alice/subnamespace/myfunc"})
	deployer.DeployFn = func(_ context.Context, f fn.Function) (res fn.DeploymentResult, err error) {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag value not seen on the Function by the deployer")
		}
		if f.Image != "example.com/alice/subnamespace/myfunc" {
			t.Fatal("image flag value not seen on the Function by deployer")
		}
		return
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// TODO it may be cleaner to error if both registry and image are provided,
	// allowing deployer implementations to avoid arbitration logic.
}

// TestDeploy_ImageFlag ensures that the image flag is used when specified.
func TestDeploy_ImageFlag(t *testing.T) {
	testImageFlag(NewDeployCmd, t)
}

func testImageFlag(cmdFn commandConstructor, t *testing.T) {
	var (
		args    = []string{"--image", "docker.io/tigerteam/foo"}
		builder = mock.NewBuilder()
	)
	root := fromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewBuildCmd(NewTestClient(fn.WithBuilder(builder)))
	cmd.SetArgs(args)

	// Execute the command
	// Should not error that registry is missing because --image was provided.
	err = cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// Now load the function and ensure that the image is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != "docker.io/tigerteam/foo" {
		t.Fatalf("Expected image to be 'docker.io/tigerteam/foo', but got '%v'", f.Image)
	}
}

// TestDeploy_ImageWithDigestErrors ensures that when an image to use is explicitly
// provided via content addressing (digest), nonsensical combinations
// of other flags (such as forcing a build or pushing being enabled), yield
// informative errors.
func TestDeploy_ImageWithDigestErrors(t *testing.T) {
	tests := []struct {
		name      string // name of the test
		image     string // value to provide as --image
		build     string // If provided, the value of the build flag
		push      bool   // if true, explicitly set argument --push=true
		errString string // the string value of an expected error
	}{
		{
			name:  "correctly formatted full image with digest yields no error (degen case)",
			image: "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			build: "false",
		},
		{
			name:      "--build forced on yields error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			build:     "true",
			errString: "building can not be enabled when using an image with digest",
		},
		{
			name:      "push flag explicitly set with digest should error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			push:      true,
			errString: "pushing is not valid when specifying an image with digest",
		},
		{
			name:      "invalid digest prefix 'Xsha256', expect error",
			image:     "example.com/myNamespace/myFunction:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "image digest 'Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e' requires 'sha256:' prefix",
		},
		{
			name:      "invalid sha hash length(added X at the end), expect error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX",
			errString: "image digest 'sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX' has an invalid sha256 hash length of 65 when it should be 64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Move into a new temp directory
			root := fromTempDirectory(t)

			// Create a new Function in the temp directory
			_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
			if err != nil {
				t.Fatal(err)
			}

			// Deploy it using the various combinations of flags from the test
			var (
				deployer  = mock.NewDeployer()
				builder   = mock.NewBuilder()
				pipeliner = mock.NewPipelinesProvider()
			)
			cmd := NewDeployCmd(NewTestClient(
				fn.WithDeployer(deployer),
				fn.WithBuilder(builder),
				fn.WithPipelinesProvider(pipeliner),
				fn.WithRegistry(TestRegistry),
			))
			args := []string{fmt.Sprintf("--image=%s", tt.image)}
			if tt.build != "" {
				args = append(args, fmt.Sprintf("--build=%s", tt.build))
			}
			if tt.push {
				args = append(args, "--push=true")
			} else {
				args = append(args, "--push=false")
			}

			cmd.SetArgs(args)
			err = cmd.Execute()
			if err != nil {
				if tt.errString == "" {
					t.Fatal(err) // no error was expected.  fail
				}
				if tt.errString != err.Error() {
					t.Fatalf("expected error '%v' not received. got '%v'", tt.errString, err.Error())
				}
				// There was an error, but it was expected
			}
		})
	}
}

// TestDeploy_ImageWithDigestDoesntPopulateBuild ensures that when --image is
// given with digest f.Build.Image is not populated because no image building
// should happen; f.Deploy.Image should be populated because the image should
// just be deployed as is (since it already has digest)
func TestDeploy_ImageWithDigestDoesntPopulateBuild(t *testing.T) {
	root := fromTempDirectory(t)
	// image with digest (well almost, atleast in length and syntax)
	const img = "docker.io/4141gauron3268@sha256:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	// Create a new Function in the temp directory
	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithRegistry(TestRegistry)))

	cmd.SetArgs([]string{"--build=false", "--push=false", "--image", img})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, _ := fn.NewFunction(root)
	if f.Build.Image != "" {
		t.Fatal("build image should be empty when deploying with digested image")
	}
	if f.Deploy.Image != img {
		t.Fatal("expected deployed image to match digested img given")
	}
}

// TestDepoy_InvalidRegistry ensures that providing an invalid registry
// fails with the expected error.
func TestDeploy_InvalidRegistry(t *testing.T) {
	testInvalidRegistry(NewDeployCmd, t)
}

func testInvalidRegistry(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	f := fn.Function{
		Root:    root,
		Name:    "myFunc",
		Runtime: "go",
	}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewTestClient())

	cmd.SetArgs([]string{"--registry=foo/bar/invald/myfunc"})

	if err := cmd.Execute(); err == nil {
		// TODO: typed ErrInvalidRegistry
		t.Fatal("invalid registry did not generate expected error")
	}
}

// TestDeploy_Namespace ensures that the namespace provided to the client
// for use when describing a function is set
// 1. The flag /env variable if provided
// 2. The namespace of the function at path if provided
// 3. The user's current active namespace
func TestDeploy_Namespace(t *testing.T) {
	root := fromTempDirectory(t)

	// A function which will be repeatedly, mockingly deployed
	f := fn.Function{Root: root, Runtime: "go", Registry: TestRegistry}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// The mock deployer responds that the given function was deployed
	// to the namespace indicated in f.Deploy.Namespace or "default" if empty
	// (it does not actually consider the current kubernetes context)
	deployer := mock.NewDeployer()

	cmd := NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, _ = fn.NewFunction(root)
	if f.Deploy.Namespace != "default" {
		t.Fatalf("expected namespace 'default', got '%v'", f.Deploy.Namespace)
	}

	// Change the function's active namespace and ensure it is used, preempting
	// the 'default' namespace from the mock
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Deploy.Namespace = "alreadyDeployed"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	cmd = NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, _ = fn.NewFunction(root)
	if f.Deploy.Namespace != "alreadyDeployed" {
		t.Fatalf("expected namespace 'alreadyDeployed', got '%v'", f.Deploy.Namespace)
	}

	// Ensure an explicit name (a flag) is taken with highest precedence
	cmd = NewDeployCmd(NewTestClient(fn.WithDeployer(deployer)))
	cmd.SetArgs([]string{"--namespace=newNamespace"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, _ = fn.NewFunction(root)
	if f.Deploy.Namespace != "newNamespace" {
		t.Fatalf("expected namespace 'newNamespace', got '%v'", f.Deploy.Namespace)
	}

}

// TestDeploy_NamespaceDefaults ensures that when not specified, a users's
// active kubernetes context is used for the namespace if available.
func TestDeploy_NamespaceDefaults(t *testing.T) {
	kubeconfig := filepath.Join(cwd(), "testdata", "TestDeploy_NamespaceDefaults/kubeconfig")
	expected := "mynamespace"
	root := fromTempDirectory(t) // clears envs and cds to empty root
	t.Setenv("KUBECONFIG", kubeconfig)

	// Create a new function
	f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Namespace != "" {
		t.Fatalf("newly created functions should not have a namespace set until deployed.  Got '%v'", f.Deploy.Namespace)
	}

	// a deployer which actually uses config.DefaultNamespace
	// This is not the default implementation of mock.NewDeployer as this would
	// be likely to be confusing, since it would vary on developer machines
	// unless they remember to clear local envs, such as is done here within
	// fromTempDirectory(t).  To avert this potential confusion, the use of
	// config.DefaultNamespace() is kept local to this test.
	deployer := mock.NewDeployer()
	deployer.DeployFn = func(_ context.Context, f fn.Function) (result fn.DeploymentResult, err error) {
		// deployer implementations shuld have integration tests which confirm
		// this logic:
		if f.Deploy.Namespace != "" {
			result.Namespace = f.Deploy.Namespace
		} else {
			result.Namespace = config.DefaultNamespace()
		}
		return
	}

	// New deploy command that will not actually deploy or build (mocked)
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(deployer),
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))
	cmd.SetArgs([]string{})

	// Execute, capturing stderr
	stderr := strings.Builder{}
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has been updated to be in namespace from the profile
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != expected { // see ./testdata/TestDeploy_NamespaceDefaults
		t.Fatalf("expected function to have active namespace '%v'.  got '%v'", expected, f.Deploy.Namespace)
	}
	// See the knative package's tests for an example of tests which require
	// the knative or kubernetes API dependency.
}

// TestDeploy_NamespaceRedeployWarning ensures that redeploying a function
// which is in a namespace other than the active namespace prints a warning.
// Implicitly checks that redeploying a previously deployed function
// results in the function being redeployed to its previous namespace if
// not instructed otherwise.
func TestDeploy_NamespaceRedeployWarning(t *testing.T) {
	// Change profile to one whose current profile is 'test-ns-deploy'
	kubeconfig := filepath.Join(cwd(), "testdata", "TestDeploy_NamespaceRedeployWarning/kubeconfig")
	root := fromTempDirectory(t)
	t.Setenv("KUBECONFIG", kubeconfig)

	// Create a Function which appears to have been deployed to 'funcns'
	f := fn.Function{
		Runtime: "go",
		Root:    root,
		Deploy:  fn.DeploySpec{Namespace: "funcns"},
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy the function without specifying namespace.
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))
	cmd.SetArgs([]string{})
	stdout := strings.Builder{}
	cmd.SetOut(&stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := "Warning: namespace chosen is 'funcns', but currently active namespace is 'mynamespace'. Continuing with deployment to 'funcns'."

	// Ensure output contained warning if changing namespace
	if !strings.Contains(stdout.String(), expected) {
		t.Log("STDOUT:\n" + stdout.String())
		t.Fatalf("Expected warning not found:\n%v", expected)
	}

	// Ensure the function was saved as having been deployed to the correct ns
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "funcns" {
		t.Fatalf("expected function to be namespace 'funcns'.  got '%v'", f.Deploy.Namespace)
	}
}

// TestDeploy_NamespaceUpdateWarning ensures that, deploying a Function
// to a new namespace issues a warning.
// Also implicitly checks that the --namespace flag takes precedence over
// the namespace of a previously deployed Function.
func TestDeploy_NamespaceUpdateWarning(t *testing.T) {
	root := fromTempDirectory(t)

	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime: "go",
		Root:    root,
		Deploy: fn.DeploySpec{
			Namespace: "myns",
		},
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy the function, specifying 'newns'
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))
	cmd.SetArgs([]string{"--namespace=newns"})
	out := strings.Builder{}
	fmt.Fprintln(&out, "Test error")
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	activeNamespace, err := k8s.GetDefaultNamespace()
	if err != nil {
		t.Fatalf("Couldnt get active namespace, got error: %v", err)
	}

	expected1 := "Info: chosen namespace has changed from 'myns' to 'newns'. Undeploying function from 'myns' and deploying new in 'newns'."
	expected2 := fmt.Sprintf("Warning: namespace chosen is 'newns', but currently active namespace is '%s'. Continuing with deployment to 'newns'.", activeNamespace)
	// Ensure output contained info and warning if changing namespace
	if !strings.Contains(out.String(), expected1) || !strings.Contains(out.String(), expected2) {
		t.Log("STDERR:\n" + out.String())
		t.Fatalf("Expected Info and/or Warning not found:\n%v|%v", expected1, expected2)
	}

	// Ensure the function was saved as having been deployed to
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "newns" {
		t.Fatalf("expected function to be deployed into namespace 'newns'.  got '%v'", f.Deploy.Namespace)
	}
}

// TestDeploy_BasicRedeploy simply ensures that redeploy works and doesnt brake
// using standard deploy method when desired namespace is deleted.
func TestDeploy_BasicRedeployInCorrectNamespace(t *testing.T) {
	root := fromTempDirectory(t)

	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime: "go",
		Root:    root,
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy the function, specifying 'newns'
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithRegistry(TestRegistry),
	))

	cmd.SetArgs([]string{"--namespace=mydns"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, _ = fn.NewFunction(root)
	if f.Deploy.Namespace == "" {
		t.Fatal("expected deployed namespace to be specified after deploy")
	}

	// get rid of desired namespace -- should still deploy as usual, now taking
	// the "already deployed" namespace
	cmd.SetArgs([]string{"--namespace="})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, _ = fn.NewFunction(root)
	if f.Namespace != "" {
		t.Fatalf("no desired namespace should be specified but is %s", f.Namespace)
	}

	if f.Deploy.Namespace == "" {
		t.Fatal("expected deployed namespace to be specified after second deploy")
	}
}

// TestDeploy_BasicRedeployPipelines simply ensures that deploy 2 times works
// and doesnt brake using pipelines
func TestDeploy_BasicRedeployPipelinesCorrectNamespace(t *testing.T) {
	root := fromTempDirectory(t)
	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime: "go",
		Root:    root,
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy the function, specifying 'newns'
	cmd := NewDeployCmd(NewTestClient(
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry(TestRegistry),
	))

	cmd.SetArgs([]string{"--remote", "--namespace=myfuncns"})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, _ = fn.NewFunction(root)
	if f.Deploy.Namespace == "" || f.Deploy.Namespace != f.Namespace {
		t.Fatalf("namespace should match desired ns when deployed - '%s' | '%s'", f.Deploy.Namespace, f.Namespace)
	}

	cmd.SetArgs([]string{"--remote", "--namespace="})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, _ = fn.NewFunction(root)
	if f.Namespace != "" {
		t.Fatal("desired ns should be empty")
	}
	if f.Deploy.Namespace != "myfuncns" {
		t.Fatalf("deployed ns should NOT have changed but is '%s'\n", f.Deploy.Namespace)
	}
}

// TestDeploy_Registry ensures that a function's registry member is kept in
// sync with the image tag.
// During normal operation (using the client API) a function's state on disk
// will be in a valid state, but it is possible that a function could be
// manually edited, necessitating some kind of consistency recovery (as
// preferable to just an error).
func TestDeploy_Registry(t *testing.T) {
	testRegistry(NewDeployCmd, t)
}

func testRegistry(cmdFn commandConstructor, t *testing.T) {
	tests := []struct {
		name             string      // name of the test
		f                fn.Function // function initial state
		args             []string    // command arguments
		expectedRegistry string      // expected value after build
		expectedImage    string      // expected value after build
	}{
		{
			// Registry function member takes precidence, updating image member
			// when out of sync.
			name: "registry member mismatch",
			f: fn.Function{
				Registry: "registry.example.com/alice",
				Build: fn.BuildSpec{
					Image: "registry.example.com/bob/f:latest",
				},
			},
			args:             []string{},
			expectedRegistry: "registry.example.com/alice",
			expectedImage:    "registry.example.com/alice/f:latest",
		},
		{
			// Registry flag takes highest precidence, affecting both the registry
			// member and the resultant image member and therefore affects subsequent
			// builds.
			name: "registry flag updates",
			f: fn.Function{
				Registry: "registry.example.com/alice",
				Build: fn.BuildSpec{
					Image: "registry.example.com/bob/f:latest",
				},
			},
			args:             []string{"--registry=registry.example.com/charlie"},
			expectedRegistry: "registry.example.com/charlie",
			expectedImage:    "registry.example.com/charlie/f:latest",
		},
		{
			// Image flag takes highest precidence, but is an override such that the
			// resultant image member is updated but the registry member is not
			// updated (subsequent builds will not be affected).
			name: "image flag overrides",
			f: fn.Function{
				Registry: "registry.example.com/alice",
				Build: fn.BuildSpec{
					Image: "registry.example.com/bob/f:latest",
				},
			},
			args:             []string{"--image=registry.example.com/charlie/f:latest"},
			expectedRegistry: "registry.example.com/alice",            // not updated
			expectedImage:    "registry.example.com/charlie/f:latest", // updated
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := fromTempDirectory(t)
			test.f.Runtime = "go"
			test.f.Name = "f"
			f, err := fn.New().Init(test.f)
			if err != nil {
				t.Fatal(err)
			}
			cmd := cmdFn(NewTestClient())
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			f, err = fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if f.Registry != test.expectedRegistry {
				t.Fatalf("expected registry '%v', got '%v'", test.expectedRegistry, f.Registry)
			}
			if f.Build.Image != test.expectedImage {
				t.Fatalf("expected image '%v', got '%v'", test.expectedImage, f.Build.Image)
			}
		})
	}
}

// TestDeploy_RegistryLoads ensures that a function with a defined registry
// will use this when recalculating .Image on deploy when no --image is
// explicitly provided.
func TestDeploy_RegistryLoads(t *testing.T) {
	testRegistryLoads(NewDeployCmd, t)
}

func testRegistryLoads(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	f := fn.Function{
		Root:     root,
		Name:     "my-func",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewTestClient(fn.WithBuilder(mock.NewBuilder()), fn.WithDeployer(mock.NewDeployer())))
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	expected := "example.com/alice/my-func:latest"
	if f.Build.Image != expected {
		t.Fatalf("expected image name '%v'. got %v", expected, f.Build.Image)
	}
}

// TestDeploy_RegistryOrImageRequired ensures that when no registry or image are
// provided (or exist on the function already), and the client has not been
// instantiated with a default registry, an ErrRegistryRequired is received.
func TestDeploy_RegistryOrImageRequired(t *testing.T) {
	testRegistryOrImageRequired(NewDeployCmd, t)
}

func testRegistryOrImageRequired(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	_, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewTestClient())

	// If neither --registry nor --image are provided, and the client was not
	// instantiated with a default registry, a ErrRegistryRequired is expected.
	cmd.SetArgs([]string{}) // this explicit clearing of args may not be necessary
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, fn.ErrRegistryRequired) {
			t.Fatalf("expected ErrRegistryRequired, got error: %v", err)
		}
	}

	// earlire test covers the --registry only case, test here that --image
	// also succeeds.
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_RemoteBuildURLPermutations ensures that the remote, build and git-url flags
// are properly respected for all permutations, including empty.
func TestDeploy_RemoteBuildURLPermutations(t *testing.T) {
	// Valid flag permutations (empty indicates flag should be omitted)
	// and a functon which will convert a permutation into flags for use
	// by the subtests.
	// The empty string indicates the case in which the flag is not provided.
	var (
		remoteValues = []string{"", "true", "false"}
		buildValues  = []string{"", "true", "false", "auto"}
		urlValues    = []string{"", "https://example.com/user/repo"}

		// toArgs converts one permutaton of the values into command arguments
		toArgs = func(remote, build, url string) []string {
			args := []string{}
			if remote != "" {
				args = append(args, fmt.Sprintf("--remote=%v", remote))
			}
			if build != "" {
				args = append(args, fmt.Sprintf("--build=%v", build))
			}
			if url != "" {
				args = append(args, fmt.Sprintf("--git-url=%v", url))
			}
			return args
		}
	)

	// returns a single test function for one possible permutation of the flags.
	newTestFn := func(remote, build, url string) func(t *testing.T) {
		return func(t *testing.T) {
			root := fromTempDirectory(t)

			// Create a new Function in the temp directory
			if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			// deploy it using the deploy command with flags set to the currently
			// effective flag permutation.
			var (
				deployer  = mock.NewDeployer()
				builder   = mock.NewBuilder()
				pipeliner = mock.NewPipelinesProvider()
			)
			cmd := NewDeployCmd(NewTestClient(
				fn.WithDeployer(deployer),
				fn.WithBuilder(builder),
				fn.WithPipelinesProvider(pipeliner),
				fn.WithRegistry(TestRegistry),
			))
			cmd.SetArgs(toArgs(remote, build, url))
			err := cmd.Execute() // err is checked below

			// Assertions
			if remote != "" && remote != "false" { // the default of "" is == false

				// REMOTE Assertions
				if !pipeliner.RunInvoked { // Remote deployer should be triggered
					t.Error("remote was not invoked")
				}
				if deployer.DeployInvoked { // Local deployer should not be triggered
					t.Error("local deployer was invoked")
				}
				if builder.BuildInvoked { // Local builder should not be triggered
					t.Error("local builder invoked")
				}

				// BUILD?
				// TODO: (enhancement) Remote deployments respect build flag values
				// of off/on/auto

				// Source Location
				// TODO: (enhancement) if git url is not provided, send local source
				// to remote deployer for use when building.

			} else {
				// LOCAL Assertions

				// TODO: (enhancement) allow --git-url when running local deployment.
				// Check that the local builder is invoked with a directive to use a
				// git repo rather than the local filesystem if building is enabled and
				// a url is provided.  For now it throws an error statign that git-url
				// is only used when --remote
				if url != "" && err == nil {
					t.Fatal("error expected when deploying from local but provided --git-url")
					return
				} else if url != "" && err != nil {
					return // test successfully confirmed this is an error case
				}

				// Remote deployer should never be triggered when deploying via local
				if pipeliner.RunInvoked {
					t.Error("remote was invoked")
				}

				// BUILD?
				if build == "" || build == "true" || build == "auto" {
					// The default case for build is auto, which is equivalent to true
					// for a newly created Function which has not yet been built.
					if !builder.BuildInvoked {
						t.Error("local builder not invoked")
					}
					if !deployer.DeployInvoked {
						t.Error("local deployer not invoked")
					}

				} else {
					// Build was explicitly disabled.
					if builder.BuildInvoked { // builder should not be invoked
						t.Error("local builder was invoked when building disabled")
					}
					if deployer.DeployInvoked { // deployer should not be invoked
						t.Error("local deployer was invoked for an unbuilt Function")
					}
					if err == nil { // Should error that it is not built
						t.Error("expected 'error: not built' not received")
					} else {
						return // test successfully confirmed this is an expected error case
					}

					// IF build was explicitly disabled, but the Function has already
					// been built, it should invoke the deployer.
					// TODO

				}

			}

			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Run all permutations
	// Run a subtest whose name is set to the args permutation tested.
	for _, remote := range remoteValues {
		for _, build := range buildValues {
			for _, url := range urlValues {
				// Run a subtest whose name is set to the args permutation tested.
				name := fmt.Sprintf("%v", toArgs(remote, build, url))
				t.Run(name, newTestFn(remote, build, url))
			}
		}
	}
}

// TestDeploy_RemotePersists ensures that the remote field is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_RemotePersists(t *testing.T) {
	root := fromTempDirectory(t)

	f, err := fn.New().Init(fn.Function{Runtime: "node", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	cmd := NewDeployCmd(NewTestClient(fn.WithRegistry(TestRegistry)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Deploy the function, specifying remote deployment(on-cluster)
	cmd = NewDeployCmd(NewTestClient(fn.WithRegistry(TestRegistry)))
	cmd.SetArgs([]string{"--remote"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Assert the function has persisted the value of remote
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if !f.Local.Remote {
		t.Fatalf("value of remote flag not persisted")
	}

	// Deploy the function again without specifying remote
	cmd = NewDeployCmd(NewTestClient(fn.WithRegistry(TestRegistry)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has retained its original value
	// (was not reset back to a flag default)
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if !f.Local.Remote {
		t.Fatalf("value of remote flag not persisted")
	}

	// Deploy the function again using a different value for remote
	viper.Reset()
	cmd.SetArgs([]string{"--remote=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has persisted the new value
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Local.Remote {
		t.Fatalf("value of remote flag not persisted")
	}
}

// TestDeploy_UnsetFlag ensures that unsetting a flag on the command
// line causes the pertinent value to be zeroed out.
func TestDeploy_UnsetFlag(t *testing.T) {
	// From a temp directory
	root := fromTempDirectory(t)

	// Create a function
	f := fn.Function{Runtime: "go", Root: root, Registry: TestRegistry}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy it, specifying a Git URL
	cmd := NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--remote", "--git-url=https://git.example.com/alice/f"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Load the function and confirm the URL was persisted
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Build.Git.URL != "https://git.example.com/alice/f" {
		t.Fatalf("url not persisted")
	}

	// Deploy it again, unsetting the value
	cmd = NewDeployCmd(NewTestClient())
	cmd.SetArgs([]string{"--git-url="})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Load the function and confirm the URL was unset
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Build.Git.URL != "" {
		t.Fatalf("url not cleared")
	}
}

// Test_ValidateBuilder tests that the bulder validation accepts the
// the set of known builders, and spot-checks an error is thrown for unknown.
func Test_ValidateBuilder(t *testing.T) {
	for _, name := range KnownBuilders() {
		if err := ValidateBuilder(name); err != nil {
			t.Fatalf("expected builder '%v' to be valid, but got error: %v", name, err)
		}
	}

	// This CLI creates no builders beyond those in the core reposiory.  Other
	// users of the client library may provide their own named implementation of
	// the fn.Builder interface. Those would have a different set of valid
	// builders.

	if err := ValidateBuilder("invalid"); err == nil {
		t.Fatalf("did not get expected error validating an invalid builder name")
	}
}

// TestReDeploy_OnRegistryChange tests that after deployed image with registry X,
// subsequent deploy with registry Y triggers build
func TestReDeploy_OnRegistryChange(t *testing.T) {
	root := fromTempDirectory(t)

	// Create a basic go Function
	f := fn.Function{
		Runtime: "go",
		Root:    root,
	}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// create build cmd
	cmdBuild := NewBuildCmd(NewTestClient(fn.WithBuilder(mock.NewBuilder())))
	cmdBuild.SetArgs([]string{"--registry=" + TestRegistry})

	// First: prebuild Function
	if err := cmdBuild.Execute(); err != nil {
		t.Fatal(err)
	}

	// change registry and deploy again
	newRegistry := "example.com/fred"

	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
	))

	cmd.SetArgs([]string{"--registry=" + newRegistry})

	// Second: Deploy with different registry and expect new build
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// ASSERT
	expectF, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal("couldnt load function from path")
	}

	if !strings.Contains(expectF.Build.Image, newRegistry) {
		t.Fatalf("expected built image '%s' to contain new registry '%s'\n", expectF.Build.Image, newRegistry)
	}
}

// TestReDeploy_OnRegistryChangeWithBuildFalse should fail with function not
// being built because the registry has changed
func TestReDeploy_OnRegistryChangeWithBuildFalse(t *testing.T) {
	root := fromTempDirectory(t)

	// Create a basic go Function
	f := fn.Function{
		Runtime: "go",
		Root:    root,
	}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// create build cmd
	cmdBuild := NewBuildCmd(NewTestClient(fn.WithBuilder(mock.NewBuilder())))
	cmdBuild.SetArgs([]string{"--registry=" + TestRegistry})

	// First: prebuild Function
	if err := cmdBuild.Execute(); err != nil {
		t.Fatal(err)
	}

	// change registry and deploy again
	newRegistry := "example.com/fred"

	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
	))

	cmd.SetArgs([]string{"--registry=" + newRegistry, "--build=false"})

	// Second: Deploy with different registry and expect 'not built' error because
	// registry has changed but build is disabled
	if err := cmd.Execute(); err == nil {
		t.Fatal(err)
	}

	// ASSERT
	expectF, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal("couldnt load function from path")
	}

	if !strings.Contains(expectF.Build.Image, TestRegistry) {
		t.Fatal("expected registry to NOT change since --build=false")
	}
}

// TestDeploy_NoErrorOnOldFunctionNotFound assures that no error is given when old Function's
// service is not available (is already deleted manually or the namespace doesnt exist etc.)
func TestDeploy_NoErrorOnOldFunctionNotFound(t *testing.T) {
	var (
		root    = fromTempDirectory(t)
		nsOne   = "nsone"
		nsTwo   = "nstwo"
		remover = mock.NewRemover()
	)

	// Simulate remover error
	remover.RemoveFn = func(n, ns string) error {
		return apiErrors.NewNotFound(schema.GroupResource{Group: "", Resource: "Namespace"}, nsOne)
	}

	// Create a basic go Function
	f := fn.Function{
		Runtime: "go",
		Root:    root,
	}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy the function to ns "nsone"
	cmd := NewDeployCmd(NewTestClient(
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover)))

	cmd.SetArgs([]string{fmt.Sprintf("--namespace=%s", nsOne)})
	err = cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// Second Deploy with different namespace
	cmd.SetArgs([]string{fmt.Sprintf("--namespace=%s", nsTwo)})

	err = cmd.Execute()

	// possible TODO: catch the os.Stderr output and check that this is printed out
	// and if this is implemented, probably change the name to *_WarnOnFunction
	// expectedWarning := fmt.Sprintf("Warning: Cant undeploy Function in namespace '%s' - service not found. Namespace/Service might be deleted already", nsOne)

	// ASSERT

	// Needs to pass since the error is set to nil for NotFound error
	if err != nil {
		t.Fatal(err)
	}
}
