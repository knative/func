package cmd

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/utils"
)

// TestCreateValidatesName ensures that the create command only accepts
// DNS-1123 labels for Function name.
func TestCreateValidatesName(t *testing.T) {
	defer fromTempDir(t)()

	// Create a new Create command with a fn.Client construtor
	// which returns a default (noop) client suitable for tests.
	cmd := NewCreateCmd(func(createConfig) *fn.Client {
		return fn.New()
	})

	// Execute the command with a function name containing invalid characters.
	cmd.SetArgs([]string{"invalid!"})
	err := cmd.Execute()

	// Confirm the expected error is returned
	var e utils.ErrInvalidFunctionName
	if !errors.As(err, &e) {
		t.Fatalf("Did not receive ErrInvalidFunctionName. Got %v", err)
	}
}

// Helpers ----

// change directory into a new temp directory.
// returned is a closure which cleans up; intended to be run as a defer:
//    defer within(t, /some/path)()
func fromTempDir(t *testing.T) func() {
	t.Helper()
	tmp := mktmp(t) // create temp directory
	owd := pwd(t)   // original working directory
	cd(t, tmp)      // change to the temp directory
	return func() { // return a deferable cleanup closure
		os.RemoveAll(tmp) // remove temp directory
		cd(t, owd)        // change director back to original
	}
}

func mktmp(t *testing.T) string {
	d, err := ioutil.TempDir("", "dir")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pwd(t *testing.T) string {
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func cd(t *testing.T, dir string) {
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}
