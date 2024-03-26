package cmd

import (
	"errors"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

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

// TestBuild_ConfigApplied ensures that the build command applies config
// settings at each level (static, global, function, envs, flags).
func TestBuild_ConfigApplied(t *testing.T) {
	testConfigApplied(NewBuildCmd, t)
}

// TestBuild_ConfigPrecedence ensures that the correct precidence for config
// are applied: static < global < function context < envs < flags
func TestBuild_ConfigPrecedence(t *testing.T) {
	testConfigPrecedence(NewBuildCmd, t)
}

// TestBuild_Default ensures that running build on a valid default Function
// (only required options populated; all else default) completes successfully.
func TestBuild_Default(t *testing.T) {
	testDefault(NewBuildCmd, t)
}

// TestBuild_FunctionContext ensures that the function contectually relevant
// to the current command execution is loaded and used for flag defaults by
// spot-checking the builder setting.
func TestBuild_FunctionContext(t *testing.T) {
	testFunctionContext(NewBuildCmd, t)
}

// TestBuild_ImageFlag ensures that the image flag is used when specified.
func TestBuild_ImageFlag(t *testing.T) {
	testImageFlag(NewBuildCmd, t)
}

// TestBuild_ImageAndRegistry ensures that image is derived when --registry
// is provided without --image; that --image is used if provided; that when
// both are provided, they are both passed to the builder; and that these
// values are persisted.
func TestBuild_ImageAndRegistry(t *testing.T) {
	testImageAndRegistry(NewBuildCmd, t)
}

// TestBuild_InvalidRegistry ensures that providing an invalid registry
// fails with the expected error.
func TestBuild_InvalidRegistry(t *testing.T) {
	testInvalidRegistry(NewBuildCmd, t)
}

// TestBuild_Registry ensures that a function's registry member is kept in
// sync with the image tag.
// During normal operation (using the client API) a function's state on disk
// will be in a valid state, but it is possible that a function could be
// manually edited, necessitating some kind of consistency recovery (as
// preferable to just an error).
func TestBuild_Registry(t *testing.T) {
	testRegistry(NewBuildCmd, t)
}

// TestBuild_RegistryLoads ensures that a function with a defined registry
// will use this when recalculating .Image on build when no --image is
// explicitly provided.
func TestBuild_RegistryLoads(t *testing.T) {
	testRegistryLoads(NewBuildCmd, t)
}

// TestBuild_RegistryOrImageRequired ensures that when no registry or image are
// provided (or exist on the function already), and the client has not been
// instantiated with a default registry, an ErrRegistryRequired is received.
func TestBuild_RegistryOrImageRequired(t *testing.T) {
	testRegistryOrImageRequired(NewBuildCmd, t)
}

// TestBuild_Authentication ensures that Token and Username/Password auth
// propagate to pushers which support them.
func TestBuild_Authentication(t *testing.T) {
	testAuthentication(NewBuildCmd, t)
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
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	var (
		builder = mock.NewBuilder()
		pusher  = mock.NewPusher()
	)
	cmd := NewBuildCmd(NewTestClient(fn.WithRegistry(TestRegistry), fn.WithBuilder(builder), fn.WithPusher(pusher)))

	cmd.SetArgs([]string{})
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

	// Execute with push enabled but with a failed build
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
