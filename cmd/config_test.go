package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"knative.dev/func/pkg/config"
)

// TestConfig_List ensures that the 'list' subcommand shows all configurable
// global settings along with their current values.
func TestConfig_List(t *testing.T) {
	root := fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", root)

	// Create a test Global Config with a customized value for registry
	os.Mkdir(filepath.Join(root, "func"), os.ModePerm)
	cfg := config.Global{
		Registry: "registry.example.com/alice",
	}
	cfg.Write(config.File())

	// Ensure the list subcommand picks it up.
	cmd := NewConfigCmd(NewClient)
	cmd.SetArgs([]string{"list", "-o=json"})

	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := struct {
		Registry string
	}{}
	json.Unmarshal(buff.Bytes(), &output)
	if output.Registry != "registry.example.com/alice" {
		t.Fatalf("did not receive expected config registry.  got '%v'", output.Registry)
	}

	// Note that fromTempDirectory completely clears the current environment
	// such that the settings from the user running tests do not interfere
	// (all environment variables are cleared), and thus the config current
	// values will all be set to their static code deafults in the config package.
	// if !strings.Contains(buff.String(), "repository")

}

// TestConfig_Set ensures that the 'set' config subcommand results in a new
// global config value being set.
func TestConfig_Add(t *testing.T) {
	root := fromTempDirectory(t)
	t.Setenv("XDG_CONFIG_HOME", root)
	if err := config.CreatePaths(); err != nil {
		t.Fatal(err)
	}

	// Add a String
	cmd := NewConfigCmd(NewClient)
	cmd.SetArgs([]string{"set", "registry", "registry.example.com/bob"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.NewDefault()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Registry != "registry.example.com/bob" {
		t.Fatalf("unexpected global config registry value: %v", cfg.Registry)
	}

	// Add a Boolean
	cmd = NewConfigCmd(NewClient)
	cmd.SetArgs([]string{"set", "verbose", "true"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	cfg, err = config.NewDefault()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Verbose != true {
		t.Fatalf("unexpected global config verbose value: %v", cfg.Registry)
	}

}
