package cmd

import (
	"errors"
	"fmt"
	"testing"

	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/mock"
)

// TestBuild_ConfigApplied ensures that the build command applies config
// settings at each level (static, global, function, envs, flags).
func TestBuild_ConfigApplied(t *testing.T) {
	testConfigApplied(NewBuildCmd, t)
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

	if err = fn.New().Init(f); err != nil {
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
	if f.Image != "registry.example.com/charlie/f:latest" {
		t.Fatalf("expected image 'registry.example.com/charlie/f:latest' got '%v'", f.Image)
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

// TestBuild_ConfigPrecedence ensures that the correct precidence for config
// are applied: static < global < function context < envs < flags
func TestBuild_ConfigPrecedence(t *testing.T) {
	testConfigPrecedence(NewBuildCmd, t)
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
	if err = fn.New().Init(f); err != nil {
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
	if err := fn.New().Init(f); err != nil {
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
	if err := fn.New().Init(f); err != nil {
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
	if err := fn.New().Init(f); err != nil {
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
	if err := fn.New().Init(f); err != nil {
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

// TestBuild_ImageFlag ensures that the image flag is used when specified.
func TestBuild_ImageFlag(t *testing.T) {
	var (
		args    = []string{"--image", "docker.io/tigerteam/foo"}
		builder = mock.NewBuilder()
	)
	root := fromTempDirectory(t)

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewBuildCmd(NewTestClient(fn.WithBuilder(builder)))
	cmd.SetArgs(args)

	// Execute the command
	// Should not error that registry is missing because --image was provided.
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

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewBuildCmd(NewTestClient())
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, fn.ErrRegistryRequired) {
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
	if err := fn.New().Init(f); err != nil {
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

// TestBuild_Registry ensures that a function's registry member is kept in
// sync with the image tag.
// During normal operation (using the client API) a function's state on disk
// will be in a valid state, but it is possible that a function could be
// manually edited, necessitating some kind of consistency recovery (as
// preferable to just an error).
func TestBuild_Registry(t *testing.T) {
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
				Image:    "registry.example.com/bob/f:latest",
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
				Image:    "registry.example.com/bob/f:latest",
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
				Image:    "registry.example.com/bob/f:latest",
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
			if err := fn.New().Init(test.f); err != nil {
				t.Fatal(err)
			}
			cmd := NewBuildCmd(NewTestClient())
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if f.Registry != test.expectedRegistry {
				t.Fatalf("expected registry '%v', got '%v'", test.expectedRegistry, f.Registry)
			}
			if f.Image != test.expectedImage {
				t.Fatalf("expected image '%v', got '%v'", test.expectedImage, f.Image)
			}
		})
	}
}

// TestBuild_FunctionContext ensures that the function contectually relevant
// to the current command execution is loaded and used for flag defaults by
// spot-checking the builder setting.
func TestBuild_FunctionContext(t *testing.T) {
	testFunctionContext(NewBuildCmd, t)
}

func testFunctionContext(cmdFn commandConstructor, t *testing.T) {
	t.Helper()
	root := fromTempDirectory(t)

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry}); err != nil {
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
	f, err := fn.NewFunction(root)
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
