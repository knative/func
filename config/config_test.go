package config_test

import (
	"path/filepath"
	"testing"

	"knative.dev/kn-plugin-func/config"

	. "knative.dev/kn-plugin-func/testing"
)

// TestNewDefaults ensures that the default Config
// constructor yelds a struct prepopulated with static
// defaults.
func TestNewDefaults(t *testing.T) {
	cfg := config.New()
	if cfg.Language != config.DefaultLanguage {
		t.Fatalf("expected config's language = '%v', got '%v'", config.DefaultLanguage, cfg.Language)
	}
}

// TestLoad ensures that loading a config reads values
// in from a config file at path.
func TestLoad(t *testing.T) {
	cfg, err := config.Load("testdata/func/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "custom" {
		t.Fatalf("loaded config did not contain values from config file.  Expected \"custom\" got \"%v\"", cfg.Language)
	}
}

// TestSave ensures that saving an update config persists.
func TestSave(t *testing.T) {
	// mktmp
	root, rm := Mktemp(t)
	defer rm()

	// touch config.yaml
	filename := filepath.Join(root, "config.yaml")

	// update
	cfg := config.New()
	cfg.Language = "testSave"

	// save
	if err := cfg.Save(filename); err != nil {
		t.Fatal(err)
	}

	// reload
	cfg, err := config.Load(filename)
	if err != nil {
		t.Fatal(err)
	}

	// assert persisted
	if cfg.Language != "testSave" {
		t.Fatalf("config did not persist.  expected 'testSave', got '%v'", cfg.Language)
	}
}

// TestPath ensures that the Path accessor returns
// XDG_CONFIG_HOME/.config/func
func TestPath(t *testing.T) {
	home := t.TempDir()                 // root of all configs
	path := filepath.Join(home, "func") // our config

	t.Setenv("XDG_CONFIG_HOME", home)

	if config.Path() != path {
		t.Fatalf("expected config path '%v', got '%v'", path, config.Path())
	}
}

// TestNewDefault ensures that the default returned from NewDefault includes
// both the static defaults (see TestNewDefaults), as well as those from the
// currently effective global config path (~/config/func).
func TestNewDefault(t *testing.T) {
	// Custom config home results in a config file default path of
	// ./testdata/func/config.yaml
	home := filepath.Join(Cwd(), "testdata")
	t.Setenv("XDG_CONFIG_HOME", home)

	cfg, err := config.NewDefault() // Should load values from above config
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Language != "custom" {
		t.Fatalf("config file not loaded")
	}
}
