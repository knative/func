package cmd

import (
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

// TestBuild_ImageFlag ensures that the image flag is used when specified.
func TestBuild_ImageFlag(t *testing.T) {
	var (
		args    = []string{"--image", "docker.io/tigerteam/foo"}
		builder = mock.NewBuilder()
	)
	root := fromTempDirectory(t)

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

// TestDeploy_RegistryOrImageRequired ensures that when no registry or image are
// provided (or exist on the function already), and the client has not been
// instantiated with a default registry, an ErrRegistryRequired is received.
func TestBuild_RegistryOrImageRequired(t *testing.T) {
	root := fromTempDirectory(t)

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

	// earlier test covers the --registry only case, test here that --image
	// also succeeds.
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_InvalidRegistry ensures that providing an invalid registry
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
	root := fromTempDirectory(t)

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

// TestBuild_UpdateImageTagIfMismatchWithRegistry ensures that the image tag is in sync with defined registry
func TestBuild_UpdateImageTagIfMismatchWithRegistry(t *testing.T) {
	var (
		builder = mock.NewBuilder()
	)
	root := fromTempDirectory(t)

	err := fn.New().Create(fn.Function{
		Runtime:  "go",
		Root:     root,
		Registry: TestRegistry,              // defined as "example.com/alice"
		Image:    "docker.io/tigerteam/foo", // image uses different registry in its tag, so it has to be updated
	})
	assert.Assert(t, err == nil)

	cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithBuilder(builder))
	}))

	err = cmd.Execute()
	assert.Assert(t, err == nil)

	f, err := fn.NewFunction(root)
	assert.Assert(t, err == nil)

	expected := TestRegistry + "/foo"
	assert.Assert(t, f.Image == expected, fmt.Sprintf("Expected image to be '"+expected+"', but got '%v'", f.Image))
}
