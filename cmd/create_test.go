package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/utils"
)

// TestCreate_Execute ensures that an invocation of create with minimal settings
// and valid input completes without error; degenerate case.
func TestCreate_Execute(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd(func(ClientOptions) *fn.Client { return fn.New() })
	cmd.SetArgs([]string{
		fmt.Sprintf("--language=%s", "go"),
	})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestCreate_NoRuntime ensures that an invocation of create must be
// done with a runtime.
func TestCreate_NoRuntime(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd(func(ClientOptions) *fn.Client { return fn.New() })
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	var e ErrNoRuntime
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrNoRuntime. Got %v", err)
	}
}

// TestCreate_WithNoRuntime ensures that an invocation of create must be
// done with one of the valid runtimes only.
func TestCreate_WithInvalidRuntime(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd(func(ClientOptions) *fn.Client { return fn.New() })
	cmd.SetArgs([]string{
		fmt.Sprintf("--language=%s", "test"),
	})

	err := cmd.Execute()
	var e ErrInvalidRuntime
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidRuntime. Got %v", err)
	}
}

// TestCreate_InvalidTemplate ensures that an invocation of create must be
// done with one of the valid templates only.
func TestCreate_InvalidTemplate(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd(func(ClientOptions) *fn.Client { return fn.New() })
	cmd.SetArgs([]string{
		fmt.Sprintf("--language=%s", "go"),
		fmt.Sprintf("--template=%s", "events"),
	})

	err := cmd.Execute()
	var e ErrInvalidTemplate
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidTemplate. Got %v", err)
	}
}

// TestCreate_ValidatesName ensures that the create command only accepts
// DNS-1123 labels for Function name.
func TestCreate_ValidatesName(t *testing.T) {
	defer fromTempDir(t)()

	// Create a new Create command with a fn.Client construtor
	// which returns a default (noop) client suitable for tests.
	cmd := NewCreateCmd(func(ClientOptions) *fn.Client { return fn.New() })

	// Execute the command with a function name containing invalid characters and
	// confirm the expected error is returned
	cmd.SetArgs([]string{"invalid!"})
	err := cmd.Execute()
	var e utils.ErrInvalidFunctionName
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidFunctionName. Got %v", err)
	}
}

// TestCreate_RepositoriesPath ensures that the create command utilizes the
// expected repositories path, respecting the setting for XDG_CONFIG_PATH
// when deriving the default
func TestCreate_RepositoriesPath(t *testing.T) {
	defer fromTempDir(t)()

	// Update XDG_CONFIG_HOME to point to some arbitrary location.
	xdgConfigHome, err := ioutil.TempDir("", "alice")
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	// The expected full path to repositories:
	expected := filepath.Join(xdgConfigHome, "func", "repositories")

	// Create command takes a function which will be invoked with the final
	// state of the createConfig, usually used to do fn.Client instantiation
	// after flags, environment variables, etc. are calculated.  In this case it
	// will validate the test condition:  that config reflects the value of
	// XDG_CONFIG_HOME, and secondarily the path suffix `func/repositories`.
	cmd := NewCreateCmd(func(cfg ClientOptions) *fn.Client {
		if cfg.RepositoriesPath != expected {
			t.Fatalf("expected repositories default path to be '%v', got '%v'", expected, cfg.RepositoriesPath)
		}
		return fn.New()
	})
	cmd.SetArgs([]string{
		fmt.Sprintf("--language=%s", "go"),
	})
	// Invoke the command, which is an airball, but does invoke the client
	// constructor, which which evaluates the aceptance condition of ensuring the
	// default repositories path was updated based on XDG_CONFIG_HOME.
	if err = cmd.Execute(); err != nil {
		t.Fatalf("unexpected error running 'create' with a default (noop) client instance: %v", err)
	}
}

// Helpers ----

// change directory into a new temp directory.
// returned is a closure which cleans up; intended to be run as a defer:
//    defer within(t, /some/path)()
func fromTempDir(t *testing.T) func() {
	t.Helper()
	tmp := mktmp(t) // create temp directory
	owd := pwd(t)   // original working directory
	cd(t, tmp)      // change to the temp directory
	return func() { // return a deferable cleanup closure
		os.RemoveAll(tmp) // remove temp directory
		cd(t, owd)        // change director back to original
	}
}

func mktmp(t *testing.T) string {
	d, err := ioutil.TempDir("", "dir")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pwd(t *testing.T) string {
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func cd(t *testing.T, dir string) {
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}
