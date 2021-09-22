package cmd

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/utils"
)

// TestCreate ensures that an invocation of create with minimal settings
// and valid input completes without error; degenerate case.
func TestCreate(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd(func(createConfig) *fn.Client { return fn.New() })
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestCreateValidatesName ensures that the create command only accepts
// DNS-1123 labels for Function name.
func TestCreateValidatesName(t *testing.T) {
	defer fromTempDir(t)()

	// Create a new Create command with a fn.Client construtor
	// which returns a default (noop) client suitable for tests.
	cmd := NewCreateCmd(func(createConfig) *fn.Client { return fn.New() })

	// Execute the command with a function name containing invalid characters and
	// confirm the expected error is returned
	cmd.SetArgs([]string{"invalid!"})
	err := cmd.Execute()
	var e utils.ErrInvalidFunctionName
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidFunctionName. Got %v", err)
	}
}

// TestCreateRepositoriesPath ensures that the create command utilizes the
// expected repositories path, respecting the setting for XDG_CONFIG_PATH
// when deriving the default
func TestCreateRepositoriesPath(t *testing.T) {
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
	cmd := NewCreateCmd(func(cfg createConfig) *fn.Client {
		if cfg.Repositories != expected {
			t.Fatalf("expected repositories default path to be '%v', got '%v'", expected, cfg.Repositories)
		}
		return fn.New()
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
