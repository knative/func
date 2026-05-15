package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWrite_FilePermissions verifies that config files are written with
// secure permissions (0600) instead of overly permissive 0777.
func TestWrite_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("File permission tests not applicable on Windows")
	}

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config and write it
	cfg := New()
	cfg.Builder = "test-builder"
	cfg.Registry = "test-registry"

	err := cfg.Write(configPath)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	mode := info.Mode().Perm()
	expectedMode := os.FileMode(0600)

	if mode != expectedMode {
		t.Errorf("Config file has incorrect permissions: got %o, want %o", mode, expectedMode)
	}

	// Verify the file is readable and writable by owner
	if mode&0400 == 0 {
		t.Error("Config file is not readable by owner")
	}
	if mode&0200 == 0 {
		t.Error("Config file is not writable by owner")
	}

	// Verify the file is NOT accessible by group or others
	if mode&0040 != 0 {
		t.Error("Config file is readable by group (should not be)")
	}
	if mode&0020 != 0 {
		t.Error("Config file is writable by group (should not be)")
	}
	if mode&0004 != 0 {
		t.Error("Config file is readable by others (should not be)")
	}
	if mode&0002 != 0 {
		t.Error("Config file is writable by others (should not be)")
	}
}

// TestCreatePaths_DirectoryPermissions verifies that config directories
// are created with appropriate permissions (0755).
func TestCreatePaths_DirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("File permission tests not applicable on Windows")
	}

	// Set up a temporary XDG_CONFIG_HOME
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create the paths
	err := CreatePaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	// Check directory permissions
	configDir := Dir()
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Failed to stat config directory: %v", err)
	}

	mode := info.Mode().Perm()
	expectedMode := os.FileMode(0755)

	if mode != expectedMode {
		t.Errorf("Config directory has incorrect permissions: got %o, want %o", mode, expectedMode)
	}

	// Check repositories directory permissions
	repoDir := RepositoriesPath()
	info, err = os.Stat(repoDir)
	if err != nil {
		t.Fatalf("Failed to stat repositories directory: %v", err)
	}

	mode = info.Mode().Perm()
	if mode != expectedMode {
		t.Errorf("Repositories directory has incorrect permissions: got %o, want %o", mode, expectedMode)
	}
}
