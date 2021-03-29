package tarfs

import (
	"os"
	"testing"
	"testing/fstest"
)

// TestEmpty ensures that an empty TarFS behaves itself.
func TestEmpty(t *testing.T) {
	f, err := os.Open("testdata/empty.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tfs, err := New(f)
	if err != nil {
		t.Fatal(err)
	}

	if err := fstest.TestFS(tfs); err != nil {
		t.Fatal(err)
	}
}

// TestFile ensures that a reader of a single file tarball proffers that file.
func TestSingle(t *testing.T) {
	f, err := os.Open("testdata/single.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tfs, err := New(f)
	if err != nil {
		t.Fatal(err)
	}

	if err := fstest.TestFS(tfs, "single.txt"); err != nil {
		t.Fatal(err)
	}
}

// TestIsNotExist ensures that a request to read a file or directory which does not
// exist returns the appropriate error.
func TestIsNotExist(t *testing.T) {
	f, err := os.Open("testdata/empty.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tfs, err := New(f)
	if err != nil {
		t.Fatal(err)
	}

	if err := fstest.TestFS(tfs, "invalid"); err == nil {
		t.Fatalf("did not receive expected error testing for a missing file")
	}
}
