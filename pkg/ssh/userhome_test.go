package ssh

import (
	"os/user"
	"runtime"
	"testing"
)

// TestUserHomeDir ensures userHomeDir returns $HOME when it is set, and the
// home directory from the user database when it is not.
func TestUserHomeDir(t *testing.T) {
	t.Run("HOME set", func(t *testing.T) {
		want := t.TempDir()
		if runtime.GOOS == "windows" {
			t.Setenv("USERPROFILE", want)
		} else {
			t.Setenv("HOME", want)
		}
		if got := userHomeDir(); got != want {
			t.Errorf("userHomeDir() = %q, want %q", got, want)
		}
	})

	t.Run("HOME not set", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("userHomeDir does not use the user database on Windows")
		}
		u, err := user.Current()
		if err != nil {
			t.Skipf("cannot look up the current user: %v", err)
		}
		t.Setenv("HOME", "")
		if got := userHomeDir(); got != u.HomeDir {
			t.Errorf("userHomeDir() = %q, want %q", got, u.HomeDir)
		}
	})
}
