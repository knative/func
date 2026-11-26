package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
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

// TestBuild_ConfigPrecedence ensures that the correct precedence for config
// are applied: static < global < function context < envs < flags
func TestBuild_ConfigPrecedence(t *testing.T) {
	testConfigPrecedence(NewBuildCmd, t)
}

// TestBuild_Default ensures that running build on a valid default Function
// (only required options populated; all else default) completes successfully.
func TestBuild_Default(t *testing.T) {
	testDefault(NewBuildCmd, t)
}

// TestBuild_FunctionContext ensures that the function contextually relevant
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

// TestBuild_BaseImage ensures that base image is used only with the right
// builders and propagates into f.Build.BaseImage
func TestBuild_BaseImage(t *testing.T) {
	testBaseImage(NewBuildCmd, t)
}

// TestBuild_BuildCAFile ensures that --build-cafile is accepted with the pack
// builder only and is remembered in the function's local settings.
func TestBuild_BuildCAFile(t *testing.T) {
	testBuildCAFile(NewBuildCmd, t)
}

// TestBuild_Push ensures that the build command properly pushes and respects
// the --push flag.
// - Push triggered after a successful build
// - Push not triggered after an unsuccessful build
// - Push can be disabled
func TestBuild_Push(t *testing.T) {
	root := FromTempDirectory(t)

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

// testBuildCAFile ensures the build CA file is read from the function's local
// settings (.func/local.yaml), the FUNC_BUILD_CAFILE env and the
// --build-cafile flag, reaches the builder, and is remembered in
// .func/local.yaml but never written to func.yaml. Given on the command with a
// builder other than pack it is an error; a remembered value is ignored there.
func testBuildCAFile(cmdFn commandConstructor, t *testing.T) {
	tests := []struct {
		name       string
		builder    string
		remembered string // value already in .func/local.yaml
		env        string // FUNC_BUILD_CAFILE
		args       []string
		want       string
		wantErr    error
	}{
		{name: "flag", builder: "pack", args: []string{"--build-cafile=certs/ca.pem"}, want: "certs/ca.pem"},
		{name: "env", builder: "pack", env: "/etc/corp/ca.pem", want: "/etc/corp/ca.pem"},
		{name: "remembered", builder: "pack", remembered: "ca.pem", want: "ca.pem"},
		{name: "flag overrides remembered", builder: "pack", remembered: "ca.pem", args: []string{"--build-cafile=other.pem"}, want: "other.pem"},
		{name: "flag clears remembered", builder: "pack", remembered: "ca.pem", args: []string{"--build-cafile="}, want: ""},
		{name: "s2i rejects flag", builder: "s2i", args: []string{"--build-cafile=ca.pem"}, wantErr: ErrBuildCAFileRequiresPack},
		{name: "s2i rejects env", builder: "s2i", env: "ca.pem", wantErr: ErrBuildCAFileRequiresPack},
		{name: "host rejects flag", builder: "host", args: []string{"--build-cafile=ca.pem"}, wantErr: ErrBuildCAFileRequiresPack},
		// A value remembered for this machine must not break other builders.
		{name: "s2i ignores remembered", builder: "s2i", remembered: "ca.pem", want: "ca.pem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := FromTempDirectory(t)
			f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry,
				Local: fn.Local{BuildCAFile: tt.remembered}})
			if err != nil {
				t.Fatal(err)
			}
			if err = f.Write(); err != nil {
				t.Fatal(err)
			}
			if tt.env != "" {
				t.Setenv("FUNC_BUILD_CAFILE", tt.env)
			}

			builder := mock.NewBuilder()
			builder.BuildFn = func(f fn.Function) error {
				if f.Local.BuildCAFile != tt.want {
					t.Errorf("expected builder to receive CA file %q, got %q", tt.want, f.Local.BuildCAFile)
				}
				return nil
			}
			cmd := cmdFn(NewTestClient(
				fn.WithBuilder(builder),
				fn.WithDeployer(mock.NewDeployer()),
				fn.WithRegistry(TestRegistry),
			))
			cmd.SetArgs(append([]string{"--builder=" + tt.builder}, tt.args...))
			err = cmd.Execute()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !builder.BuildInvoked {
				t.Fatal("builder was not invoked")
			}
			if f, err = fn.NewFunction(root); err != nil {
				t.Fatal(err)
			}
			if f.Local.BuildCAFile != tt.want {
				t.Fatalf("expected .func/local.yaml buildCAFile %q, got %q", tt.want, f.Local.BuildCAFile)
			}
			funcYAML, err := os.ReadFile(filepath.Join(root, fn.FunctionFile))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(string(funcYAML)), "cafile") {
				t.Fatalf("the CA file must not be written to func.yaml:\n%s", funcYAML)
			}
		})
	}
}
