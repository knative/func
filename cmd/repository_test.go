package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepository_List ensures that the 'list' subcommand shows the client's
// set of repositories by name for builtin repositories, by explicitly
// setting the repositories path to a new path which includes no others.
func TestRepository_List(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", newTempDir()) // use tmp dir for repos
	cmd := NewRepositoryListCmd()

	// Execute the command, capturing the output sent to stdout
	stdout := piped(t)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the output matches expectd (whitespace trimmed)
	expect := "default"
	output := stdout()
	if output != expect {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expect, output)
	}
}

// TestRepository_Add ensures that the 'add' subcommand accepts its positional
// arguments, respects the repositories path flag, and the expected name is echoed
// upon subsequent 'list'.
func TestRepository_Add(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", newTempDir()) // use tmp dir for repos
	var (
		add    = NewRepositoryAddCmd()
		list   = NewRepositoryListCmd()
		stdout = piped(t)
	)

	// add [flags] <old> <new>
	add.SetArgs([]string{
		"newrepo",
		testRepoURI("repository", t),
	})

	// Parse flags and args, performing action
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// List post-add, capturing output from stdout
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the list output now includes the name from args (whitespace trimmed)
	expect := "default\nnewrepo"
	output := stdout()
	if output != expect {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expect, output)
	}
}

// TestRepository_Rename ensures that the 'rename' subcommand accepts its
// positional arguments, respects the repositories path flag, and the name is
// reflected as having been reanamed upon subsequent 'list'.
func TestRepository_Rename(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", newTempDir()) // use tmp dir for repos
	var (
		add    = NewRepositoryAddCmd()
		rename = NewRepositoryRenameCmd()
		list   = NewRepositoryListCmd()
		stdout = piped(t)
	)

	// add a repo which will be renamed
	add.SetArgs([]string{"newrepo", testRepoURI("repository", t)})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// rename [flags] <old> <new>
	rename.SetArgs([]string{
		"newrepo",
		"renamed",
	})

	// Parse flags and args, performing action
	if err := rename.Execute(); err != nil {
		t.Fatal(err)
	}

	// List post-rename, capturing output from stdout
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the list output now includes the name from args (whitespace trimmed)
	expect := "default\nrenamed"
	output := stdout()
	if output != expect {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expect, output)
	}
}

// TestReposotory_Remove ensures that the 'remove' subcommand accepts name as
// its argument, respects the repositorieis flag, and the entry is removed upon
// subsequent 'list'.
func TestRepository_Remove(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", newTempDir()) // use tmp dir for repos
	var (
		add    = NewRepositoryAddCmd()
		remove = NewRepositoryRemoveCmd()
		list   = NewRepositoryListCmd()
		stdout = piped(t)
	)

	// add a repo which will be removed
	add.SetArgs([]string{"newrepo", testRepoURI("repository", t)})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// remove [flags] <name>
	remove.SetArgs([]string{
		"newrepo",
	})

	// Parse flags and args, performing action
	if err := remove.Execute(); err != nil {
		t.Fatal(err)
	}

	// List post-remove, capturing output from stdout
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the list output now includes the name from args (whitespace trimmed)
	expect := "default"
	output := stdout()
	if output != expect {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expect, output)
	}
}

// Helpers
// -------

// pipe the output of stdout to a buffer whose value is returned
// from the returned function.  Call pipe() to start piping output
// to the buffer, call the returned function to access the data in
// the buffer.
func piped(t *testing.T) func() string {
	t.Helper()
	var (
		o = os.Stdout
		c = make(chan error, 1)
		b strings.Builder
	)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stdout = w

	go func() {
		_, err := io.Copy(&b, r)
		r.Close()
		c <- err
	}()

	return func() string {
		os.Stdout = o
		w.Close()
		err := <-c
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(b.String())
	}
}

// TEST REPO URI:  Return URI to repo in ./testdata of matching name.
// Suitable as URI for repository override. returns in form file://
// Must be called prior to mktemp in tests which changes current
// working directory as it depends on a relative path.
// Repo uri:  file://$(pwd)/testdata/repository.git (unix-like)
//            file: //$(pwd)\testdata\repository.git (windows)
func testRepoURI(name string, t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	repo := filepath.Join(cwd, "testdata", name+".git")
	return fmt.Sprintf(`file://%s`, filepath.ToSlash(repo))
}

// create a temp dir.  prefixed 'func'.  panic on fail.
// TODO: check if this is a duplicate:
func newTempDir() string {
	path, err := ioutil.TempDir("", "func")
	if err != nil {
		panic(err)
	}
	return path
}
