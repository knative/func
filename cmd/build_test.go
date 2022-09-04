package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
	. "knative.dev/kn-plugin-func/testing"
)

// TestBuild_ImageFlag ensures that the image flag is used when specified.
func TestBuild_ImageFlag(t *testing.T) {
	var (
		args    = []string{"--image", "docker.io/tigerteam/foo"}
		builder = mock.NewBuilder()
	)

	root, cleanup := Mktemp(t)
	defer cleanup()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}

	cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithBuilder(builder))
	}))

	// Execute the command
	cmd.SetArgs(args)
	err := cmd.Execute()
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

// TestBuild_RegistryRequired ensures that when no registry is provided, and
// the client has not been instantiated with a default registry, an
// ErrRegistryRequired is received.
func TestBuild_RegistryRequired(t *testing.T) {
	t.Helper()
	root, rm := Mktemp(t)
	defer rm()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	cmd.SetArgs([]string{}) // this explicit clearing of args may not be necessary
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, ErrRegistryRequired) {
			t.Fatalf("expected ErrRegistryRequired, got error: %v", err)
		}
	}
}

// TestBuild_InvalidRegistry ensures that providing an invalid resitry
// fails with the expected error.
func TestBuild_InvalidRegistry(t *testing.T) {
	testInvalidRegistry(NewBuildCmd, t)
}

// TestBuild_RegistryLoads ensures that a function with a defined registry
// will use this when recalculating .Image on build when no --image is
// explicitly provided.
func TestBuild_RegistryLoads(t *testing.T) {
	testRegistryLoads(NewBuildCmd, t)
}

// TestBuild_BuilderPersists ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestBuild_BuilderPersists(t *testing.T) {
	testBuilderPersists(NewBuildCmd, t)
}

// TestBuild_ValidateBuilder ensures that the validation function correctly
// identifies valid and invalid builder short names.
func TestBuild_BuilderValidated(t *testing.T) {
	testBuilderValidated(NewBuildCmd, t)
}

// TestBuild_Push ensures that the build command properly pushes and respects
// the --push flag.
// - Push triggered after a successful build
// - Push not triggered after an unsuccessful build
// - Push can be disabled
func TestBuild_Push(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	var (
		builder = mock.NewBuilder()
		pusher  = mock.NewPusher()
		cmd     = NewBuildCmd(NewClientFactory(func() *fn.Client {
			return fn.New(fn.WithRegistry(TestRegistry), fn.WithBuilder(builder), fn.WithPusher(pusher))
		}))
	)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert there was no push
	if pusher.PushInvoked {
		t.Fatal("push should not be invoked by default")
	}

	// Execute with push enabled
	cmd.SetArgs([]string{"--push"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert there was a push
	if !pusher.PushInvoked {
		t.Fatal("push should be invoked when requested and a successful build")
	}

	// Exeute with push enabled but with a failed build
	builder.BuildFn = func(f fn.Function) error {
		return errors.New("mock error")
	}
	pusher.PushInvoked = false
	_ = cmd.Execute() // expected error

	// Assert push was not invoked when the build failed
	if pusher.PushInvoked {
		t.Fatal("push should not be invoked on a failed build")
	}
}

// TestBuild_registryConfigurationInYaml tests that a build will execute successfully
// when there is no registry provided on the command line, but one exists in func.yaml
func TestBuild_registryConfigurationInYaml(t *testing.T) {
	var (
		builder = mock.NewBuilder() // with a mock builder
	)

	// Run this test in a temporary directory
	defer Fromtemp(t)()
	// Write a func.yaml config which does not specify an image
	// but does specify a registry
	funcYaml := `name: registrytest
namespace: ""
runtime: go
image: ""
registry: quay.io/boson/foo
created: 2021-01-01T00:00:00+00:00
`
	if err := ioutil.WriteFile("func.yaml", []byte(funcYaml), 0600); err != nil {
		t.Fatal(err)
	}

	// Create build command that will use a mock builder.
	cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithBuilder(builder), fn.WithRegistry("quay.io/boson/foo"))
	}))

	// Execute the command
	err := cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}
}

func testBuilderPersistence(t *testing.T, testRegistry string, cmdBuilder func(ClientFactory) *cobra.Command) {
	//add this to work with all other tests in deploy_test.go
	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()

	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRegistry(testRegistry))

	f := fn.Function{Runtime: "go", Root: root, Name: "myfunc", Registry: testRegistry}

	if err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	cmd := cmdBuilder(NewClientFactory(func() *fn.Client {
		return client
	}))

	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var err error
	// Assert the function has persisted a value of builder (has a default)
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Builder == "" {
		t.Fatal("value of builder not persisted")
	}

	// Build the function, specifying a Builder
	cmd.SetArgs([]string{"--builder=s2i"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// Assert the function has persisted the value of builder
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Builder != "s2i" {
		t.Fatal("value of builder flag not persisted when provided")
	}
	// Build the function without specifying a Builder
	cmd = cmdBuilder(NewClientFactory(func() *fn.Client {
		return client
	}))
	cmd.SetArgs([]string{"--registry", testRegistry})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has retained its original value
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}

	if f.Builder != "pack" {
		t.Fatal("value of builder updated when not provided")
	}

	// Build the function again using a different builder
	cmd.SetArgs([]string{"--builder=pack"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has persisted the new value
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Builder != "pack" {
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

// TestBuild_BuilderPersistence ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestBuild_BuilderPersistence(t *testing.T) {
	testBuilderPersistence(t, "docker.io/tigerteam", NewBuildCmd)
}

// Test_ValidateBulder ensures that the builder validation
// function pre-validates for all builder types supported by
// this CLI.
func Test_validateBuilder(t *testing.T) {
	tests := []struct {
		name      string
		builder   string
		wantError bool
	}{
		{
			name:      "valid builder - pack",
			builder:   "pack",
			wantError: false,
		},
		{
			name:      "valid builder - s2i",
			builder:   "s2i",
			wantError: false,
		},
		{
			name:      "invalid builder",
			builder:   "foo",
			wantError: true,
		},
		{
			name:      "builder not specified - invalid option",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuilder(tt.builder)
			if tt.wantError != (err != nil) {
				t.Errorf("ValidateBuilder() = Wanted error %v but actually got %v", tt.wantError, err)
			}
		})
	}
}
