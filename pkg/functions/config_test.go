package functions_test

// Config tests do not have private access, as they are testing manifest
// behavior of the public package interface.  For implementation tests, see
// files suffixed '_unit_test.go'.

import (
	"os"
	"path/filepath"
	"testing"

	"knative.dev/func/pkg/config"
)

// TestConfig_PathDefault ensures that config defaults to XDG_CONFIG_HOME/func
// and that a config.yaml placed there is picked up by NewDefault.
func TestConfig_PathDefault(t *testing.T) {
	tmp := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfgDir := filepath.Join(tmp, "func")
	if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	cfgFile := filepath.Join(cfgDir, config.Filename)
	content := "builder: test-builder\nregistry: quay.io/test\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault returned error: %v", err)
	}
	if cfg.Builder != "test-builder" {
		t.Fatalf("expected Builder 'test-builder', got %q", cfg.Builder)
	}
	if cfg.Registry != "quay.io/test" {
		t.Fatalf("expected Registry 'quay.io/test', got %q", cfg.Registry)
	}
}

// TestConfig_Path ensures that the config file specified via FUNC_CONFIG_FILE
// is respected by NewDefault / File().
func TestConfig_Path(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, "myconfig.yaml")
	content := "builder: explicit-builder\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("FUNC_CONFIG_FILE", cfgFile)

	if got := config.File(); got != cfgFile {
		t.Fatalf("expected config.File() to be %q, got %q", cfgFile, got)
	}

	cfg, err := config.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault returned error: %v", err)
	}
	if cfg.Builder != "explicit-builder" {
		t.Fatalf("expected Builder 'explicit-builder', got %q", cfg.Builder)
	}
}

// TestConfig_RepositoriesPath ensures that CreatePaths creates the
// repositories directory under the effective config path.
func TestConfig_RepositoriesPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	reposPath := filepath.Join(tmp, "func", config.Repositories)
	if _, err := os.Stat(reposPath); !os.IsNotExist(err) {
		_ = os.RemoveAll(reposPath)
	}

	if err := config.CreatePaths(); err != nil {
		t.Fatalf("CreatePaths returned error: %v", err)
	}

	fi, err := os.Stat(reposPath)
	if err != nil {
		t.Fatalf("expected repositories path to exist, stat error: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("expected repositories path to be a directory: %s", reposPath)
	}
}

