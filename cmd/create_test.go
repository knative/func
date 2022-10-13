package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"knative.dev/func/utils"
)

// TestCreate_Execute ensures that an invocation of create with minimal settings
// and valid input completes without error; degenerate case.
func TestCreate_Execute(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"--language", "go", "myfunc"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestCreate_NoRuntime ensures that an invocation of create must be
// done with a runtime.
func TestCreate_NoRuntime(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"myfunc"}) // Do not use test command args

	err := cmd.Execute()
	var e ErrNoRuntime
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrNoRuntime. Got %v", err)
	}
}

// TestCreate_WithNoRuntime ensures that an invocation of create must be
// done with one of the valid runtimes only.
func TestCreate_WithInvalidRuntime(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"--language", "invalid", "myfunc"})

	err := cmd.Execute()
	var e ErrInvalidRuntime
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidRuntime. Got %v", err)
	}
}

// TestCreate_InvalidTemplate ensures that an invocation of create must be
// done with one of the valid templates only.
func TestCreate_InvalidTemplate(t *testing.T) {
	_ = fromTempDirectory(t)

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"--language", "go", "--template", "invalid", "myfunc"})

	err := cmd.Execute()
	var e ErrInvalidTemplate
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidTemplate. Got %v", err)
	}
}

// TestCreate_ValidatesName ensures that the create command only accepts
// DNS-1123 labels for function name.
func TestCreate_ValidatesName(t *testing.T) {
	_ = fromTempDirectory(t)

	// Execute the command with a function name containing invalid characters and
	// confirm the expected error is returned
	cmd := NewCreateCmd(NewClient)
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
	_ = fromTempDirectory(t)

	// Update XDG_CONFIG_HOME to point to some arbitrary location which we know
	// The above call to fromTempDirectory also updates, but value is not returned.
	xdgConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	// The expected full path is XDG_CONFIG_HOME/func/repositories
	expected := filepath.Join(xdgConfigHome, "func", "repositories")

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"myfunc"}) // Do not use test command args
	cfg, err := newCreateConfig(cmd, []string{}, NewClient)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.RepositoriesPath != expected {
		t.Fatalf("expected repositories default path to be '%v', got '%v'", expected, cfg.RepositoriesPath)
	}
}

// TestCreate_ConfigOptional ensures that the system can be used without
// any additional configuration being required.
func TestCreate_ConfigOptional(t *testing.T) {
	_ = fromTempDirectory(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := NewCreateCmd(NewClient)
	cmd.SetArgs([]string{"--language=go", "myfunc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Not failing is success.  Config files or settings beyond what are
	// automatically written to to the given config home are currently optional.
}
