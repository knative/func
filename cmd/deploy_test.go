package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"k8s.io/utils/pointer"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
	"knative.dev/kn-plugin-func/mock"
	. "knative.dev/kn-plugin-func/testing"
)

const TestRegistry = "example.com/alice"

type commandConstructor func(ClientFactory) *cobra.Command

// TestDeploy_RegistryOrImageRequired ensures that when no registry or image are
// provided (or exist on the function already), and the client has not been
// instantiated with a default registry, an ErrRegistryRequired is received.
func TestDeploy_RegistryOrImageRequired(t *testing.T) {
	testRegistryOrImageRequired(NewDeployCmd, t) // shared with build
}

func testRegistryOrImageRequired(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root, rm := Mktemp(t)
	defer rm()

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
	root, rm := Mktemp(t)
	defer rm()

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
	root, rm := Mktemp(t)
	defer rm()

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
	root, rm := Mktemp(t)
	defer rm()

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

	if f.Image != "example.com/alice/myFunc:latest" {
		t.Fatalf("unexpected image name: %v", f.Image)
	}
}

// TestDeploy_BuilderPersists ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_BuilderPersists(t *testing.T) {
	testBuilderPersists(NewDeployCmd, t)
}

func testBuilderPersists(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root, rm := Mktemp(t)
	defer rm()

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
	if f.Builder == "" {
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
	if f.Builder != builders.S2I {
		t.Fatal("value of builder flag not persisted when provided")
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
	if f.Builder != builders.S2I {
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
	if f.Builder != builders.Pack {
		t.Fatal("value of builder flag not persisted on subsequent build")
	}

	// Build the function, specifying a platform with "pack" Builder
	cmd.SetArgs([]string{"--platform", "linux"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error")
	}

	// Set an invalid builder
	cmd.SetArgs([]string{"--builder", "invalid"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error")
	}
}

// TestDeploy_BuilderValidated ensures that the validation function correctly
// identifies valid and invalid builder short names.
func TestDeploy_BuilderValidated(t *testing.T) {
	testBuilderValidated(NewDeployCmd, t)
}

func testBuilderValidated(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root, rm := Mktemp(t)
	defer rm()

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

// TestDeploy_ValidateBuilder tests that the ValidateBuilder validator
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

func Test_runDeploy(t *testing.T) {
	tests := []struct {
		name                 string
		gitURL               string
		gitBranch            string
		gitDir               string
		buildType            string
		funcFile             string
		expectFileURL        *string
		expectFileBranch     *string
		expectFileContextDir *string
		expectCallURL        *string
		expectCallBranch     *string
		expectCallContextDir *string
		errString            string
	}{
		{
			name:      "Git arguments don't get saved to func.yaml but are used in the pipeline invocation",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git",
			gitBranch: "main",
			gitDir:    "func",
			buildType: fn.BuildTypeGit,
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
		},
		{
			name:      "Git url gets split when in the format url#branch",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git#main",
			gitDir:    "func",
			buildType: fn.BuildTypeGit,
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
		},
		{
			name:      "Git arguments override func.yaml but don't get saved",
			gitURL:    "git@github.com:knative-sandbox/kn-plugin-func.git",
			gitBranch: "main",
			gitDir:    "func",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00
build: git
git:
  url: git@github.com:my-repo/my-function.git
  revision: master
  contextDir: pwd`,
			expectCallURL:        pointer.StringPtr("git@github.com:knative-sandbox/kn-plugin-func.git"),
			expectCallBranch:     pointer.StringPtr("main"),
			expectCallContextDir: pointer.StringPtr("func"),
			expectFileURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectFileBranch:     pointer.StringPtr("master"),
			expectFileContextDir: pointer.StringPtr("pwd"),
		},
		{
			name: "Git properties work without arguments",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00
build: git
git:
  url: git@github.com:my-repo/my-function.git
  revision: master
  contextDir: pwd`,
			expectFileURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectFileBranch:     pointer.StringPtr("master"),
			expectFileContextDir: pointer.StringPtr("pwd"),
			expectCallURL:        pointer.StringPtr("git@github.com:my-repo/my-function.git"),
			expectCallBranch:     pointer.StringPtr("master"),
			expectCallContextDir: pointer.StringPtr("pwd"),
		},
		{
			name:      "check error when providing git flags with buildType local",
			gitURL:    "git@github.com:my-repo/my-function.git",
			buildType: "local",
			funcFile: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			errString: "remote git arguments require the --build=git flag",
		},
	}

	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captureFn fn.Function
			pipeline := &mock.PipelinesProvider{
				RunFn: func(f fn.Function) error {
					captureFn = f
					return nil
				},
			}
			deployer := mock.NewDeployer()
			defer Fromtemp(t)()
			cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
				return fn.New(
					fn.WithPipelinesProvider(pipeline),
					fn.WithDeployer(deployer))
			}))
			cmd.SetArgs([]string{}) // Do not use test command args

			// TODO: the below viper.SetDefault calls appear to be altering
			// the default values of flags as a way set various values of flags.
			// This could perhaps be better achieved by constructing an array
			// of flag arguments, set via cmd.SetArgs(...).  This would more directly
			// test the use-case of flag values (as opposed to the indirect proxy
			// of their defaults), and would avoid the need to call viper.Reset() to
			// avoid affecting other tests.
			viper.SetDefault("git-url", tt.gitURL)
			viper.SetDefault("git-branch", tt.gitBranch)
			viper.SetDefault("git-dir", tt.gitDir)
			viper.SetDefault("build", tt.buildType)
			viper.SetDefault("registry", "docker.io/tigerteam")
			defer viper.Reset()

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				if tt.errString == "" {
					t.Fatalf("Problem executing command: %v", err)
				} else if err := err.Error(); tt.errString != err {
					t.Fatalf("Error expected to be %v but was %v", tt.errString, err)
				}
			}

			fileFunction, err := fn.NewFunction(".")

			if err != nil {
				t.Fatalf("problem creating function: %v", err)
			}

			{
				if fileURL, expectedURL := pointer.StringPtrDerefOr(fileFunction.Git.URL, ""), pointer.StringPtrDerefOr(tt.expectFileURL, ""); fileURL != expectedURL {
					t.Fatalf("file Git URL expected to be (%v) but was (%v)", expectedURL, fileURL)
				}
				if fileBranch, expectedBranch := pointer.StringPtrDerefOr(fileFunction.Git.Revision, ""), pointer.StringPtrDerefOr(tt.expectFileBranch, ""); fileBranch != expectedBranch {
					t.Fatalf("file Git branch expected to be (%v) but was (%v)", expectedBranch, fileBranch)
				}
				if fileDir, expectedDir := pointer.StringPtrDerefOr(fileFunction.Git.ContextDir, ""), pointer.StringPtrDerefOr(tt.expectFileContextDir, ""); fileDir != expectedDir {
					t.Fatalf("file Git contextDir expected to be (%v) but was (%v)", expectedDir, fileDir)
				}
			}

			{
				if caputureURL, expectedURL := pointer.StringPtrDerefOr(captureFn.Git.URL, ""), pointer.StringPtrDerefOr(tt.expectCallURL, ""); caputureURL != expectedURL {
					t.Fatalf("call Git URL expected to be (%v) but was (%v)", expectedURL, caputureURL)
				}
				if captureBranch, expectedBranch := pointer.StringPtrDerefOr(captureFn.Git.Revision, ""), pointer.StringPtrDerefOr(tt.expectCallBranch, ""); captureBranch != expectedBranch {
					t.Fatalf("call Git Branch expected to be (%v) but was (%v)", expectedBranch, captureBranch)
				}
				if captureDir, expectedDir := pointer.StringPtrDerefOr(captureFn.Git.ContextDir, ""), pointer.StringPtrDerefOr(tt.expectCallContextDir, ""); captureDir != expectedDir {
					t.Fatalf("call Git Dir expected to be (%v) but was (%v)", expectedDir, captureDir)
				}
			}
		})
	}
}

func Test_imageWithDigest(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		buildType string
		pushBool  bool
		funcFile  string
		errString string
	}{
		{
			name:      "valid full name with digest, expect success",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "valid image name, build not 'disabled', expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			buildType: "local",
			errString: "the --build flag 'local' is not valid when using --image with digest",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "valid image name, --push specified, expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			pushBool:  true,
			errString: "the --push flag 'true' is not valid when using --image with digest",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "invalid digest prefix, expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "value 'docker.io/4141gauron3268/static_test_digest:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e' in --image has invalid prefix syntax for digest (should be 'sha256:')",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "invalid sha hash length(added X at the end), expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX",
			errString: "sha256 hash in 'sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX' from --image has the wrong length (65), should be 64",
			funcFile: `name: test-func
runtime: go`,
		},
	}

	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployer := mock.NewDeployer()
			cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
				return fn.New(
					fn.WithDeployer(deployer))
			}))

			// Set flags manually & reset after.
			// Differs whether build was set via CLI (gives an error if not 'disabled')
			// or not (prints just a warning)
			if tt.buildType == "" {
				cmd.SetArgs([]string{
					fmt.Sprintf("--image=%s", tt.image),
					fmt.Sprintf("--push=%t", tt.pushBool),
				})
			} else {
				cmd.SetArgs([]string{
					fmt.Sprintf("--image=%s", tt.image),
					fmt.Sprintf("--build=%s", tt.buildType),
					fmt.Sprintf("--push=%t", tt.pushBool),
				})
			}
			defer cmd.ResetFlags()

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				if err := err.Error(); tt.errString != err {
					t.Fatalf("Error expected to be (%v) but was (%v)", tt.errString, err)
				}
			}
		})
	}
}

func Test_checkNamespaceDeploy(t *testing.T) {
	tests := []struct {
		name          string
		confNamespace string
		funcNamespace string
		context       bool
		expected      string
		expectedError string
	}{
		{
			name: "defaults and no context returns empty with no error",
		},
		{
			name:     "defaults with a context returns the context",
			context:  true,
			expected: "test-ns-deploy", // from ./testdata
		},
		{
			name:          "extant value takes precidence when none provided",
			confNamespace: "",
			funcNamespace: "last-deploy-value",
			context:       true,
			expected:      "last-deploy-value",
		},
		{
			name:          "config values take precidence",
			confNamespace: "flag-value",
			funcNamespace: "last-deploy-value",
			context:       true,
			expected:      "flag-value",
		},
	}

	// contains a kube config with active namespace "test-ns-deploy"
	contextPath := fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd())

	defer WithEnvVar(t, "KUBECONFIG", contextPath)()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			root, rm := Mktemp(t)
			defer rm()

			if test.context {
				defer WithEnvVar(t, "KUBECONFIG", contextPath)()
			} else {
				defer WithEnvVar(t, "KUBECONFIG", cwd())()
			}

			f := fn.Function{
				Runtime:   "go",
				Root:      root,
				Namespace: test.funcNamespace,
			}

			if err := fn.New().Create(f); err != nil {
				t.Fatal(err)
			}

			ns, err := checkNamespaceDeploy(test.funcNamespace, test.confNamespace)
			if err != nil {
				t.Fatal(err)
			}
			if ns != test.expected {
				t.Errorf("expected namespace '%v' got '%v'", test.expected, ns)
			}

		})
	}
}

func Test_namespaceCheck(t *testing.T) {
	tests := []struct {
		name      string
		registry  string
		namespace string
		funcFile  string
		expectNS  string
	}{
		{
			name:     "first deployment(no ns in func.yaml), not given via cli, expect write in func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "test-ns-deploy",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:     "ns in func.yaml, not given via cli, current ns matches func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "test-ns-deploy",
			funcFile: `name: test-func
namespace: "test-ns-deploy"
runtime: go`,
		},
		{
			name:      "ns in func.yaml, given via cli (always override)",
			namespace: "test-ns-deploy",
			expectNS:  "test-ns-deploy",
			registry:  "docker.io/4141gauron3268",
			funcFile: `name: test-func
namespace: "non-default"
runtime: go`,
		},
		{
			name:     "ns in func.yaml, not given via cli, current ns does NOT match func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "non-default",
			funcFile: `name: test-func
namespace: "non-default"
runtime: go`,
		},
	}

	// create mock kubeconfig with set namespace as 'default'
	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			deployer := mock.NewDeployer()
			defer Fromtemp(t)()
			cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
				return fn.New(
					fn.WithDeployer(deployer))
			}))

			// set namespace argument if given & reset after
			cmd.SetArgs([]string{}) // Do not use test command args
			viper.SetDefault("namespace", tt.namespace)
			viper.SetDefault("registry", tt.registry)
			defer viper.Reset()

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				t.Fatalf("Got error '%s' but expected success", err)
			}

			fileFunction, err := fn.NewFunction(".")
			if err != nil {
				t.Fatalf("problem creating function: %v", err)
			}

			if fileFunction.Namespace != tt.expectNS {
				t.Fatalf("Expected namespace '%s' but function has '%s' namespace", tt.expectNS, fileFunction.Namespace)
			}
		})
	}
}
