package kafkaruntime

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtract exercises the extract-and-cache path. It only runs when a binary
// is embedded for the current platform (i.e. `make kafka-runtime-embed` ran
// before the test build); otherwise Extract returns ErrUnavailable and the test
// is skipped, which is the expected state on a bare build.
func TestExtract(t *testing.T) {
	// Isolate the config dir so the test does not touch the user's real one.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := Extract()
	if err == ErrUnavailable {
		t.Skip("no adapter binary embedded for this platform; run `make kafka-runtime-embed`")
	}
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Lives under the version-keyed config dir.
	wantDir := filepath.Join(tmp, "func", "kafka-runtime", Version)
	if filepath.Dir(path) != wantDir {
		t.Errorf("extracted to %q, want dir %q", path, wantDir)
	}

	// Is executable.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("extracted binary is not executable: mode %v", fi.Mode())
	}

	// Second call is a cache hit and returns the same path.
	path2, err := Extract()
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}
	if path2 != path {
		t.Errorf("second Extract returned %q, want cached %q", path2, path)
	}
}
