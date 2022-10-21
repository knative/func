package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"knative.dev/func/config"

	. "knative.dev/func/testing"
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
// in from a config file at path, and in this case (unlike NewDefault) the
// file must exist at path or error.
func TestLoad(t *testing.T) {
	cfg, err := config.Load("testdata/func/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "custom" {
		t.Fatalf("loaded config did not contain values from config file.  Expected \"custom\" got \"%v\"", cfg.Language)
	}

	// and ensure error
	cfg, err = config.Load("invalid/path")
	if err == nil {
		t.Fatal("did not receive expected error loading nonexistent config path")
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
	if err := cfg.Write(filename); err != nil {
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

	if config.Dir() != path {
		t.Fatalf("expected config path '%v', got '%v'", path, config.Dir())
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

// TestCreatePaths ensures that the paths are created when requested.
func TestCreatePaths(t *testing.T) {
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)

	t.Setenv("XDG_CONFIG_HOME", home)

	if err := config.CreatePaths(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(config.Dir()); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("config path '%v' not created", config.Dir())
		}
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(config.Dir(), "repositories")); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("config path '%v' not created", config.Dir())
		}
		t.Fatal(err)
	}

	// Trying to create when repositories path is invalid should error
	_ = os.WriteFile("./invalidRepositoriesPath.txt", []byte{}, os.ModePerm)
	t.Setenv("FUNC_REPOSITORIES_PATH", "./invalidRepositoriesPath.txt")
	if err := config.CreatePaths(); err == nil {
		t.Fatal("did not receive error when creating paths with an invalid FUNC_REPOSITORIES_PATH")
	}

	// Trying to Create config path should bubble errors, for example when HOME is
	// set to a nonexistent path.
	_ = os.WriteFile("./invalidConfigHome.txt", []byte{}, os.ModePerm)
	t.Setenv("XDG_CONFIG_HOME", "./invalidConfigHome.txt")
	if err := config.CreatePaths(); err == nil {
		t.Fatal("did not receive error when creating paths in an invalid home")
	}

}

// TestNewDefault_ConfigNotRequired ensures that when creating a new
// config which would load a global config, its nonexistence causes no error.
func TestNewDefault_ConfigNotRequired(t *testing.T) {
	// Custom config home results in a config file default path of
	// ./testdata/func/config.yaml
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", home)

	_, err := config.NewDefault() // Should not error despite no config.
	if err != nil {
		t.Fatal(err)
	}
}

// TestRepositoriesPath returns the path expected
// (XDG_CONFIG_HOME/func/repositories by default)
func TestRepositoriesPath(t *testing.T) {
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", home)

	expected := filepath.Join(home, "func", config.Repositories)
	if config.RepositoriesPath() != expected {
		t.Fatalf("unexpected reposiories path: %v", config.RepositoriesPath())
	}
}

// TestWrite ensures that a config is written to the given path and errors
// are returned correctly when the path does not exist.
func TestWrite(t *testing.T) {
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", home)

	cfg := config.New()
	cfg.Language = "example"

	// First try writing to a nonexistent path
	if err := cfg.Write(config.File()); err == nil {
		t.Fatal("did not receive error writing to a nonexistent path")
	}

	// Create the path and try again
	if err := config.CreatePaths(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Write(config.File()); err != nil {
		t.Fatal(err)
	}
}
