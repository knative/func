package cmd_test

import (
	"testing"

	"github.com/lkingland/faas/cmd"
)

// TestNewRoot ensures that NewRoot returns a non-nil command with at least a name that
// prints help (has no run statement).
func TestRootCommand(t *testing.T) {
	root := cmd.NewRoot("")

	if root == nil {
		t.Fatal("returned a nil command")
	}
	if root.Name() == "" {
		t.Fatal("root command's name was not set")
	}
	if root.Run != nil || root.RunE != nil {
		t.Fatal("root command should print usage, but has a run function set")
	}
}

// TestVersion ensures that the root command sets the passed version.
func TestVersion(t *testing.T) {
	v := "v0.0.0"
	root := cmd.NewRoot(v)
	if root.Version != v {
		t.Fatalf("expected root command to have version '%v', got '%v'", v, root.Version)
	}
}
