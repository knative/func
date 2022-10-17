package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/mock"
)

const TestRegistry = "example.com/alice"

type commandConstructor func(ClientFactory) *cobra.Command

// TestDeploy_Default ensures that running deploy on a valid default Function
// (only required options populated; all else default) completes successfully.
func TestDeploy_Default(t *testing.T) {
	root := fromTempDirectory(t)

	// A Function with the minimum required values for deployment populated.
	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Deploy using an instance of the deploy command which uses a fully default
	// (noop filled) Client.  Execution should complete without error.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
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

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	// If neither --registry nor --image are provided, and the client was not
	// instantiated with a default registry, a ErrRegistryRequired is expected.
	cmd.SetArgs([]string{}) // this explicit clearing of args may not be necessary
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, ErrRegistryRequired) {
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

// TestBuild_ImageAndRegistry ensures that image is derived when --registry
// is provided without --image; that --image is used if provided; that when
// both are provided, they are both passed to the deployer; and that these
// values are persisted.
func TestDeploy_ImageAndRegistry(t *testing.T) {
	testImageAndRegistry(NewDeployCmd, t)
}

func testImageAndRegistry(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	var (
		builder  = mock.NewBuilder()
		deployer = mock.NewDeployer()
		cmd      = cmdFn(NewClientFactory(func() *fn.Client {
			return fn.New(
				fn.WithBuilder(builder),
				fn.WithDeployer(deployer),
				fn.WithRegistry(TestRegistry))
		}))
	)

	// If only --registry is provided:
	// the resultant Function should have the registry populated and image
	// derived from the name.
	cmd.SetArgs([]string{"--registry=example.com/alice"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag not provided to deployer")
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If only --image is provided:
	// the deploy should not fail, and the resultant Function should have the
	// Image member set to what was explicitly provided via the --image flag
	// (not a derived name)
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Image != "example.com/alice/myfunc" {
			t.Fatalf("deployer expected f.Image 'example.com/alice/myfunc', got '%v'", f.Image)
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If both --registry and --image are provided:
	// they should both be plumbed through such that downstream agents (deployer
	// in this case) see them set on the Function and can act accordingly.
	cmd.SetArgs([]string{"--registry=example.com/alice", "--image=example.com/alice/subnamespace/myfunc"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag value not seen on the Function by the deployer")
		}
		if f.Image != "example.com/alice/subnamespace/myfunc" {
			t.Fatal("image flag value not seen on the Function by deployer")
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// TODO it may be cleaner to error if both registry and image are provided,
	// allowing deployer implementations to avoid arbitration logic.
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
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	cmd.SetArgs([]string{"--registry=foo/bar/invald/myfunc"})

	if err := cmd.Execute(); err == nil {
		// TODO: typed ErrInvalidRegistry
		t.Fatal("invalid registry did not generate expected error")
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
		Name:     "myFunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithDeployer(mock.NewDeployer()))
	}))
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	expected := "example.com/alice/myFunc:latest"
	if f.Image != expected {
		t.Fatalf("expected image name '%v'. got %v", expected, f.Image)
	}
}

// TestDeploy_BuilderPersists ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_BuilderPersists(t *testing.T) {
	testBuilderPersists(NewDeployCmd, t)
}

func testBuilderPersists(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	t.Setenv("KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))
	root := fromTempDirectory(t)

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}
	cmd := cmdFn(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithRegistry(TestRegistry))
	}))
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

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := cmdFn(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	cmd.SetArgs([]string{"--builder=invalid"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error string '%v', got '%v'", "expected", err.Error())
	}

}

// Test_ValidateBuilder tests that the bulder validation accepts the
// accepts === the set of known builders.
func Test_ValidateBuilder(t *testing.T) {
	for _, name := range builders.All() {
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

// TestDeploy_RemoteBuildURLPermutations ensures that the remote, build and git-url flags
// are properly respected for all permutations, including empty.
func TestDeploy_RemoteBuildURLPermutations(t *testing.T) {
	// Valid flag permutations (empty indicates flag should be omitted)
	// and a functon which will convert a permutation into flags for use
	// by the subtests.
	var (
		remoteValues = []string{"", "true", "false"}
		buildValues  = []string{"", "true", "false", "auto"}
		urlValues    = []string{"", "https://example.com/user/repo"}

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
			if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			// deploy it using the deploy commnand with flags set to the currently
			// effective flag permutation.
			var (
				deployer  = mock.NewDeployer()
				builder   = mock.NewBuilder()
				pipeliner = mock.NewPipelinesProvider()
				cmd       = NewDeployCmd(NewClientFactory(func() *fn.Client {
					return fn.New(
						fn.WithDeployer(deployer),
						fn.WithBuilder(builder),
						fn.WithPipelinesProvider(pipeliner),
						fn.WithRegistry(TestRegistry),
					)
				}))
			)
			cmd.SetArgs(toArgs(remote, build, url))
			err := cmd.Execute()

			// Assertions
			if remote != "" && remote != "false" { // default "" is == false.
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

// Test_ImageWithDigestErrors ensures that when an image to use is explicitly
// provided via content addressing (digest), nonsensical combinations
// of other flags (such as forcing a build or pushing being enabled), yield
// informative errors.
func Test_ImageWithDigestErrors(t *testing.T) {
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
			errString: "image 'example.com/myNamespace/myFunction:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e' has an invalid prefix syntax for digest (should be 'sha256:')",
		},
		{
			name:      "invalid sha hash length(added X at the end), expect error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX",
			errString: "sha256 hash in 'sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX' has the wrong length (65), should be 64",
		},
	}

	t.Setenv("KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Move into a new temp directory
			root := fromTempDirectory(t)

			// Create a new Function in the temp directory
			if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			// Deploy it using the various combinations of flags from the test
			var (
				deployer  = mock.NewDeployer()
				builder   = mock.NewBuilder()
				pipeliner = mock.NewPipelinesProvider()
				cmd       = NewDeployCmd(NewClientFactory(func() *fn.Client {
					return fn.New(
						fn.WithDeployer(deployer),
						fn.WithBuilder(builder),
						fn.WithPipelinesProvider(pipeliner),
						fn.WithRegistry(TestRegistry),
					)
				}))
			)
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
			err := cmd.Execute()
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

// Test_namespace ensures that the combinations of
// a configured (provided via flag or env variable) namespace
// takes highest precidence, the previously existing namespace
// on the function has second precidence, the namespace
// from the current context (if available) is used next, and finally
// the default namespace.
func Test_namespace(t *testing.T) {
	tests := []struct {
		name          string
		confNamespace string
		funcNamespace string
		context       bool
		expected      string
	}{
		{
			name:          "flag or env takes highest precidence",
			confNamespace: "conf-ns",
			funcNamespace: "func-ns",
			context:       true,
			expected:      "conf-ns",
		},
		{
			name:          "preexisting func namespace takes second precidence",
			confNamespace: "",
			funcNamespace: "func-ns",
			context:       true,
			expected:      "func-ns",
		},
		{
			name:          "namespace from active context is default if available",
			confNamespace: "",
			funcNamespace: "",
			context:       true,
			expected:      "test-ns-deploy", // see ./testdata
		},
		{
			name:          "default",
			confNamespace: "",
			funcNamespace: "",
			context:       false,
			expected:      "",
		},
	}

	// contains a kube config with active namespace "test-ns-deploy"
	contextPath := fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd())

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Helper()
			root := fromTempDirectory(t)

			// if running with an active kubeconfig
			if test.context {
				t.Setenv("KUBECONFIG", contextPath)
			} else {
				t.Setenv("KUBECONFIG", cwd())
			}

			// Creat a funcction which may be already deployed
			// (have a namespace)
			f := fn.Function{
				Runtime: "go",
				Root:    root,
				Deploy: fn.DeploySpec{
					Namespace: test.funcNamespace,
				},
			}
			if err := fn.New().Create(f); err != nil {
				t.Fatal(err)
			}

			ns := namespace(deployConfig{Namespace: test.confNamespace}, f, os.Stderr)
			if ns != test.expected {
				t.Errorf("expected namespace '%v' got '%v'", test.expected, ns)
			}

		})
	}
}

/*
Test_namespaceCheck cases were refactored into:
			"first deployment(no ns in func.yaml), not given via cli, expect write in func.yaml"
			AKA: Undeployed Function, deploying with no ns specified: use defaults
			See TestDeploy_NamespaceDefaults

			"ns in func.yaml, not given via cli, current ns matches func.yaml",
			AKA: Function Deployed, should redeploy to the same namespace.
			See TestDeploy_NamespaceRedeployWarning

			"ns in func.yaml, given via cli (always override)",
			AKA: Function Deployed, but should deploy wherever --namespace says
			See TestDeploy_NamespaceUpdateWarning which confirms this case exists
			  and yields a warning message.

			"ns in func.yaml, not given via cli, current ns does NOT match func.yaml",
			AKA: A previously deployed function should stay in its namespace, even
			  when the user's active namespace differs.
			See TestDeploy_NamespaceRedeployWarning which confirms this case exists
			  and yields a warning message.


*/

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
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Deploy the Function specifying all of the git-related flags
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithPipelinesProvider(mock.NewPipelinesProvider()), fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir, "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Load the Function and ensure the flags were stored.
	f, err := fn.NewFunction(root)
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
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// A Pipelines Provider which will validate the expected values were received
	pipeliner := mock.NewPipelinesProvider()
	pipeliner.RunFn = func(f fn.Function) error {
		if f.Build.Git.URL != url {
			t.Errorf("Pipeline Provider expected git URL '%v' got '%v'", url, f.Build.Git.URL)
		}
		if f.Build.Git.Revision != branch {
			t.Errorf("Pipeline Provider expected git branch '%v' got '%v'", branch, f.Build.Git.Revision)
		}
		if f.Build.Git.ContextDir != dir {
			t.Errorf("Pipeline Provider expected git dir '%v' got '%v'", url, f.Build.Git.ContextDir)
		}
		return nil
	}

	// Deploy the Function specifying all of the git-related flags and --remote
	// such that the mock pipelines provider is invoked.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithPipelinesProvider(pipeliner), fn.WithRegistry(TestRegistry))
	}))

	cmd.SetArgs([]string{"--remote=true", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_GitURLBranch ensures that a --git-url which specifies the branch
// in the URL is equivalent to providing --git-branch
func TestDeploy_GitURLBranch(t *testing.T) {
	root := fromTempDirectory(t)

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	var (
		url            = "https://example.com/user/repo#branch"
		expectedUrl    = "https://example.com/user/repo"
		expectedBranch = "branch"
	)
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
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

// TestDeploy_NamespaceDefaults ensures that when not specified, a users's
// active kubernetes context is used for the namespace if available.
func TestDeploy_NamespaceDefaults(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(cwd(), "testdata", "kubeconfig_deploy_namespace"))
	root := fromTempDirectory(t)

	// Create a new function
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Assert it has no default namespace set
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatalf("newly created functions should not have a namespace set until deployed.  Got '%v'", f.Deploy.Namespace)
	}

	// New deploy command that will not actually deploy or build (mocked)
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
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
	if f.Deploy.Namespace != "test-ns-deploy" { // from testdata/kubeconfig_deploy_namespace
		t.Fatalf("expected function to have active namespace 'test-ns-deploy' by default.  got '%v'", f.Deploy.Namespace)
	}
	// See the knative package's tests for an example of tests which require
	// the knative or kubernetes API dependency.
}

// TestDeploy_NamespaceUpdateWarning ensures that, deploying a Function
// to a new namespace issues a warning.
// Also implicitly checks that the --namespace flag takes precidence over
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
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Redeploy the function, specifying 'newns'
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--namespace=newns"})
	out := strings.Builder{}
	fmt.Fprintln(&out, "Test errpr")
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	expected := "Warning: function is in namespace 'myns', but requested namespace is 'newns'. Continuing with deployment to 'newns'."

	// Ensure output contained warning if changing namespace
	if !strings.Contains(out.String(), expected) {
		t.Log("STDERR:\n" + out.String())
		t.Fatalf("Expected warning not found:\n%v", expected)
	}

	// Ensure the function was saved as having been deployed to
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "newns" {
		t.Fatalf("expected function to be deoployed into namespace 'newns'.  got '%v'", f.Deploy.Namespace)
	}

}

// TestDeploy_NamespaceRedeployWarning ensures that redeploying a function
// which is in a namespace other than the active namespace prints a warning.
// Implicitly checks that redeploying a previously deployed function
// results in the function being redeployed to its previous namespace if
// not instructed otherwise.
func TestDeploy_NamespaceRedeployWarning(t *testing.T) {
	// Change profile to one whose current profile is 'test-ns-deploy'
	t.Setenv("KUBECONFIG", filepath.Join(cwd(), "testdata", "kubeconfig_deploy_namespace"))
	root := fromTempDirectory(t)

	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime: "go",
		Root:    root,
		Deploy:  fn.DeploySpec{Namespace: "myns"},
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Redeploy the function without specifying namespace.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{})
	stderr := strings.Builder{}
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := "Warning: Function is in namespace 'myns', but currently active namespace is 'test-ns-deploy'. Continuing with redeployment to 'myns'."

	// Ensure output contained warning if changing namespace
	if !strings.Contains(stderr.String(), expected) {
		t.Log("STDERR:\n" + stderr.String())
		t.Fatalf("Expected warning not found:\n%v", expected)
	}

	// Ensure the function was saved as having been deployed to
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "myns" {
		t.Fatalf("expected function to be updated with namespace 'myns'.  got '%v'", f.Deploy.Namespace)
	}
}
