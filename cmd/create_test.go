package cmd

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"knative.dev/kn-plugin-func/utils"
)

// TestCreate_Execute ensures that an invocation of create with minimal settings
// and valid input completes without error; degenerate case.
func TestCreate_Execute(t *testing.T) {
	defer fromTempDir(t)()

	// command with a client factory which yields a fully default client.
	cmd := NewCreateCmd()
	cmd.SetArgs([]string{"--language", "go"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestCreate_NoRuntime ensures that an invocation of create must be
// done with a runtime.
func TestCreate_NoRuntime(t *testing.T) {
	defer fromTempDir(t)()

	cmd := NewCreateCmd()
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

	cmd := NewCreateCmd()
	cmd.SetArgs([]string{"--language", "invalid"})

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

	cmd := NewCreateCmd()
	cmd.SetArgs([]string{
		"--language", "go",
		"--template", "invalid",
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

	// Execute the command with a function name containing invalid characters and
	// confirm the expected error is returned
	cmd := NewCreateCmd()
	cmd.SetArgs([]string{"invalid!"})
	err := cmd.Execute()
	var e utils.ErrInvalidFunctionName
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidFunctionName. Got %v", err)
	}
}

// TestCreateConfig_RepositoriesPath ensures that the create command utilizes
// the expected repositories path, respecting the setting for XDG_CONFIG_PATH
// when deriving the default
func TestCreateConfig_RepositoriesPath(t *testing.T) {
	defer fromTempDir(t)()

	// Update XDG_CONFIG_HOME to point to some arbitrary location.
	xdgConfigHome, err := ioutil.TempDir("", "alice")
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	// The expected full path is XDG_CONFIG_HOME/func/repositories
	expected := filepath.Join(xdgConfigHome, "func", "repositories")

	cmd := NewCreateCmd()
	cfg := createConfig{}
	cfg, err = newCreateConfig(cmd, []string{})

	if cfg.Repositories != expected {
		t.Fatalf("expected repositories default path to be '%v', got '%v'", expected, cfg.Repositories)
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
