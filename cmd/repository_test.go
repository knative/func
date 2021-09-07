package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"knative.dev/kn-plugin-func/mock"
)

// TestRepositoryList ensures that the 'list' subcommand shows the client's
// set of repositories by name, respects the repositories flag (provides it to
// the client), and prints the list as expected.
func TestRepositoryList(t *testing.T) {
	var (
		client = mock.NewClient()
		list   = NewRepositoryListCmd(testRepositoryClientFn(client))
	)

	// Set the repositories flag, which will be passed to the client instance
	// in the form of a config.
	list.SetArgs([]string{"--repositories=testpath"})

	// Execute the command, capturing the output sent to stdout
	stdout := piped(t)
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the repository flag setting was preserved during execution
	if client.RepositoriesPath != "testpath" {
		t.Fatal("repositories flag not passed to client")
	}

	// Assert the output matches expectd (whitespace trimmed)
	expect := "default"
	output := stdout()
	if output != expect {
		t.Fatalf("expected:\n'%v'\ngot:\n'%v'\n", expect, output)
	}
}

// TestRepositoryAdd ensures that the 'add' subcommand accepts its positional
// arguments, respects the repositories path flag, and the expected name is echoed
// upon subsequent 'list'.
func TestRepositoryAdd(t *testing.T) {
	var (
		client = mock.NewClient()
		add    = NewRepositoryAddCmd(testRepositoryClientFn(client))
		list   = NewRepositoryListCmd(testRepositoryClientFn(client))
		stdout = piped(t)
	)
	// add [flags] <old> <new>
	add.SetArgs([]string{
		"--repositories=testpath",
		"newrepo",
		"https://git.example.com/user/repo",
	})

	// Parse flags and args, performing action
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the repositories flag was parsed and provided to client
	if client.RepositoriesPath != "testpath" {
		t.Fatal("repositories flag not passed to client")
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

// TestRepositoryRename ensures that the 'rename' subcommand accepts its
// positional arguments, respects the repositories path flag, and the name is
// reflected as having been reanamed upon subsequent 'list'.
func TestRepositoryRename(t *testing.T) {
	var (
		client = mock.NewClient()
		add    = NewRepositoryAddCmd(testRepositoryClientFn(client))
		rename = NewRepositoryRenameCmd(testRepositoryClientFn(client))
		list   = NewRepositoryListCmd(testRepositoryClientFn(client))
		stdout = piped(t)
	)

	// add a repo which will be renamed
	add.SetArgs([]string{"newrepo", "https://git.example.com/user/repo"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// rename [flags] <old> <new>
	rename.SetArgs([]string{
		"--repositories=testpath",
		"newrepo",
		"renamed",
	})

	// Parse flags and args, performing action
	if err := rename.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the repositories flag was parsed and provided to client
	if client.RepositoriesPath != "testpath" {
		t.Fatal("repositories flag not passed to client")
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

// TestReposotoryRemove ensures that the 'remove' subcommand accepts name as
// its argument, respects the repositorieis flag, and the entry is removed upon
// subsequent 'list'.
func TestRepositoryRemove(t *testing.T) {
	var (
		client = mock.NewClient()
		add    = NewRepositoryAddCmd(testRepositoryClientFn(client))
		remove = NewRepositoryRemoveCmd(testRepositoryClientFn(client))
		list   = NewRepositoryListCmd(testRepositoryClientFn(client))
		stdout = piped(t)
	)

	// add a repo which will be removed
	add.SetArgs([]string{"newrepo", "https://git.example.com/user/repo"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// remove [flags] <name>
	remove.SetArgs([]string{
		"--repositories=testpath",
		"newrepo",
	})

	// Parse flags and args, performing action
	if err := remove.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the repositories flag was parsed and provided to client
	if client.RepositoriesPath != "testpath" {
		t.Fatal("repositories flag not passed to client")
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

// testClientFn returns a repositoryClientFn which always returns the provided
// mock client.  The client may have various funciton implementations overriden
// so as to test particular cases, and a config object is created from the
// flags and environment variables in the same way as the actual commands, with
// the effective value recorded on the mock as members for test assertions.
func testRepositoryClientFn(client *mock.Client) repositoryClientFn {
	c := testRepositoryClient{client} // type gymnastics
	return func(args []string) (repositoryClientConfig, RepositoryClient, error) {
		cfg, err := newRepositoryConfig(args)
		client.Confirm = cfg.Confirm
		client.RepositoriesPath = cfg.Repositories
		if err != nil {
			return cfg, c, err
		}
		return cfg, c, nil
	}
}

type testRepositoryClient struct{ *mock.Client }

func (c testRepositoryClient) Repositories() Repositories {
	return Repositories(c.Client.Repositories())
}

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
