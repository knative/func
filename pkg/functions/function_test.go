//go:build !integration
// +build !integration

package functions_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-cmp/cmp"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestFunction_PathDefault ensures that the default path when instantiating
// a NewFunciton is to use the current working directory.
func TestFunction_PathDefault(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	var f fn.Function
	var err error

	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	f.Name = "f"
	f.Runtime = "go"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(""); err != nil {
		t.Fatal(err)
	}
	if f.Name != "f" {
		t.Fatalf("expected function 'f', got '%v'", f.Name)
	}
}

// TestFunction_PathErrors ensures that instantiating a function errors if
// the path does not exist or is not a directory, but does not require the
// path contain an initialized function.
func TestFunction_PathErrors(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	_, err := fn.NewFunction(root)
	if err != nil {
		t.Fatalf("an empty but valid directory path should not error. got '%v'", err)
	}

	_, err = fn.NewFunction(filepath.Join(root, "nonexistent"))
	if err == nil {
		t.Fatalf("a nonexistent path should error")
	}

	if err := os.WriteFile("filepath", []byte{}, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	_, err = fn.NewFunction(filepath.Join(root, "filepath"))
	if err == nil {
		t.Fatalf("an invalid path (non-directory) should error")
	}

}

// TestFunction_WriteIdempotency ensures that a function can be written repeatedly
// without change.
func TestFunction_WriteIdempotency(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a function
	f := fn.Function{
		Runtime: TestRuntime,
		Root:    root,
	}
	_, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Load the function and write it again
	f1, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := f1.Write(); err != nil {
		t.Fatal(err)
	}

	// Load it again and compare
	f2, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(f1, f2); diff != "" {
		t.Error("function differs after reload (-before, +after):", diff)
	}
}

// TestFunction_NameDefault ensures that a function's name is defaulted to that
// which can be derived from the last part of its path.
// Creating a new function from a path will error if there is no function at
// that path.  Creating using the client initializes the default.
func TestFunction_NameDefault(t *testing.T) {
	// A path at which there is no function currently
	root := "testdata/testFunctionNameDefault"
	defer Using(t, root)()
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Initialized() {
		t.Fatal("a function about an empty, but valid path, shold not be initialized")
	}

	// Create the function at the path
	client := fn.New(fn.WithRegistry(TestRegistry))
	f = fn.Function{
		Runtime: TestRuntime,
		Root:    root,
	}
	f, err = client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the name was defaulted as expected
	if f.Name != "testFunctionNameDefault" {
		t.Fatalf("expected name 'testFunctionNameDefault', got '%v'", f.Name)
	}
}

// Test_Interpolate ensures environment variable interpolation processes
// environment variables by interpolating properly formatted references to
// local environment variables, returning a final simple map structure.
// Also ensures that nil value references are interpreted as meaning the
// environment is not to be included in the resultant map, rather than included
// with an empty value.
// TODO: Perhaps referring to a nonexistent local env var should be treated
// as a "leave as is" (do not set) rather than "required" resulting in error?
// TODO: What use case does a nil pointer in the Env struct serve?  Add it
// explicitly here ore get rid of the nils.
func Test_Interpolate(t *testing.T) {
	t.Setenv("INTERPOLATE", "interpolated")
	cases := []struct {
		Value    string
		Expected string
		Error    bool
	}{
		// Simple values are kept unchanged
		{Value: "simple value", Expected: "simple value"},
		// Properly referenced environment variables are interpolated
		{Value: "{{ env:INTERPOLATE }}", Expected: "interpolated"},
		// Other interpolation types other than "env" are left unchanged
		{Value: "{{ other:TYPE }}", Expected: "{{ other:TYPE }}", Error: false},
		// Properly formatted references to missing variables error
		{Value: "{{ env:MISSING }}", Expected: "", Error: true},
	}

	name := "NAME" // default name for all tests
	for _, c := range cases {
		t.Logf("Value: %v\n", c.Value)
		var (
			envs    = []fn.Env{{Name: &name, Value: &c.Value}} // pre-interpolated
			vv, err = fn.Interpolate(envs)                     // interpolated
			v       = vv[name]                                 // final value
		)
		if c.Error && err == nil {
			t.Fatal("expected error in Envs interpolation not received")
		}
		if v != c.Expected {
			t.Fatalf("expected env value '%v' to be interpolated as '%v', but got '%v'", c.Value, c.Expected, v)
		}
	}

	// Nil value should be treated as being disincluded from the resultant map.
	envs := []fn.Env{{Name: &name}} // has a nil *Value ptr
	vv, err := fn.Interpolate(envs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vv) != 0 {
		t.Fatalf("expected envs with a nil value to not be included in interpolation result")
	}
}

// TestFunction_MarshallingError check that the correct error gets reported back to the
// user if the function that is being loaded is failing marshalling and cannot be migrated
func TestFunction_MarshallingError(t *testing.T) {
	root := "testdata/testFunctionMarshallingError"

	// Load the function to see it fail with a marshalling error
	_, err := fn.NewFunction(root)
	if err != nil {
		if !strings.Contains(err.Error(), "Marshalling: 'func.yaml' is not valid:") {
			t.Fatalf("expected unmarshalling error")
		}

	}
}

// TestFunction_MigrationError check that the correct error gets reported back to the
// user if the function that is being loaded is failing marshalling and cannot be migrated
func TestFunction_MigrationError(t *testing.T) {
	root := "testdata/testFunctionMigrationError"

	// Load the function to see it fail with a migration error
	_, err := fn.NewFunction(root)
	if err != nil {
		// This function makes the migration fails
		if !strings.Contains(err.Error(), "migration 'migrateToBuilderImages' error") {
			t.Fatalf("expected migration error")
		}
	}

}

// TestFunction_Built ensures that the function's Built method reports
// filesystem changes as indicating the function is no longer Built (aka stale)
// This includes modifying timestamps, removing or adding files.
func TestFunction_Built(t *testing.T) {
	var (
		ctx      = context.Background()
		builder  = mock.NewBuilder()
		client   = fn.New(fn.WithBuilder(builder), fn.WithRegistry(TestRegistry))
		testfile = "example.go"
		root, rm = Mktemp(t)
	)
	defer rm()

	// Create and build a function, which also stamps.
	f, err := client.Init(fn.Function{Runtime: TestRuntime, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}

	// Prior to a filesystem edit, it will be Built.
	if !f.Built() {
		t.Fatal("freshly built function reported Built==false (1)")
	}

	// Release thread and wait to ensure that the clock advances even in constrained CI environments
	time.Sleep(100 * time.Millisecond)

	// Edit the filesystem by touching a file (updating modified timestamp)
	if err := os.Chtimes(filepath.Join(root, "func.yaml"), time.Now(), time.Now()); err != nil {
		fmt.Println(err)
	}

	// Release thread and wait to ensure that the clock advances even in constrained CI environments
	time.Sleep(100 * time.Millisecond)

	if f.Built() {
		t.Fatal("client did not detect file timestamp change as indicating build staleness")
	}

	// Build and double-check Built has been reset
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}
	if !f.Built() {
		t.Fatal("freshly built function reported Built==false (2)")
	}

	// Edit the function's filesystem by adding a file.
	file, err := os.Create(filepath.Join(root, testfile))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	// The system should now detect the function is stale
	if f.Built() {
		t.Fatal("client did not detect an added file as indicating build staleness")
	}

	// Build and double-check Built has been reset
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}
	if !f.Built() {
		t.Fatal("freshly built function reported Built==false (3)")
	}

	// Remove the testfile, which should result in the client reporting that
	// the function is no longer Built (stale)
	if err := os.Remove(filepath.Join(root, testfile)); err != nil {
		t.Fatal(err)
	}
	if f.Built() {
		t.Fatal("client did not detect a removed file as indicating build staleness")
	}
}

// TestFunction_Stamp ensures that the Stamp method and it's associated
// accessor BuildStamp:
//
//		yields an empty string if the function is unbuilt
//		yields a build stamp once built
//		The value is unchanged on multiple invocations with an unchanged fs.
//		The value changes if the filesystem changes.
//	 Creates a journal when requested.
func TestFunction_Stamp(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	f := fn.Function{Root: root, Runtime: "go", Name: "f"}
	client := fn.New(fn.WithBuilder(mock.NewBuilder()), fn.WithRegistry(TestRegistry))
	stamp := f.BuildStamp()

	// In-memory functions should have no buildstamp
	if stamp != "" {
		t.Fatalf("build stamp of an uninitialized function should be '', got '%v'", stamp)
	}

	// Initialized (but not built) functions should also have no stamp
	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}
	stamp = f.BuildStamp()
	if stamp != "" {
		t.Fatalf("initial build stamp of an unbuilt but initialized function should be empty, got '%v'", stamp)
	}

	// Built functions should have a stamp
	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	stamp = f.BuildStamp()
	if stamp == "" {
		t.Fatal("building the function did not yield a build stamp")
	}

	// Explicitly stamping again should have no effect
	if err = f.Stamp(); err != nil {
		t.Fatal(err)
	}
	stamp2 := f.BuildStamp()
	if stamp2 != stamp {
		t.Fatalf("re-stamping an unchanged function changed its stamp.  expected '%v', got '%v'", stamp, stamp2)
	}

	// Windows is randomly failing the following test.  This is a quick
	// way to confirm it's a racing condition with fs modification.
	// Test succeeds reliably on linux, and there is an explicit .Flush
	time.Sleep(1 * time.Second)

	// Editing the filesystem and re-stamping should have an effect
	if err := os.Chtimes(filepath.Join(root, "func.yaml"), time.Now(), time.Now()); err != nil {
		fmt.Println(err)
	}
	if err = f.Stamp(); err != nil {
		t.Fatal(err)
	}
	stamp2 = f.BuildStamp()
	if stamp2 == "" {
		t.Fatal("stamping a built function which has had disk changes since build resulted in an empty stamp.")
	}
	if stamp2 == stamp {
		t.Fatalf("stamping a changed function did not change stamp.  got '%v' again", stamp2)
	}

	// Asking to stamp again with a journal should result in there being
	// a "[timestamp]built.log" file in .func
	if err = f.Stamp(fn.WithStampJournal()); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Join(root, fn.RunDataDir))
	if err != nil {
		t.Fatal(err)
	}

	createdJournal := false
	rx := regexp.MustCompile(`^\d{4}.*built\.log$`)
	for _, file := range files {
		if rx.MatchString(file.Name()) {
			createdJournal = true
			break
		}
	}
	if !createdJournal {
		t.Fatal("expected journal log not found")
	}
}

// TestFunction_Local checks if writing a function with custom Local spec
// stays the same for the current system. The test does the following:
//
//	create a new function
//	set Local.Remote to true
//	write it to the disk
//	load it again into a new function object
//
// The load should be successful and Local.Remote should be true
func TestFunction_Local(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	fConfig := fn.Function{Root: root, Runtime: "go", Name: "f"}
	client := fn.New(fn.WithBuilder(mock.NewBuilder()), fn.WithRegistry(TestRegistry))
	f, err := client.Init(fConfig)
	if err != nil {
		t.Fatal(err)
	}
	f.Local.Remote = true

	err = f.Write()
	if err != nil {
		t.Fatal(err)
	}

	// Load the function from the same location
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if !f.Local.Remote {
		t.Fatal("expected remote flag to be set")
	}
}

// TestFunction_LocalTransient ensures that the Local field is transient and
// is not serialised in a way that affects other clones of the function.
// The test does the following:
//
//	create a function (with Local.Remote set)
//	push the function to a remote repo (locally setup for the test)
//	clone the function from the remote repo into a new location
//
// The new function should not have Local.Remote set (as it is a transient field)
func TestFunction_LocalTransient(t *testing.T) {

	// Initialise a new function
	root, rm := Mktemp(t)
	defer rm()

	fConfig := fn.Function{Root: root, Runtime: "go", Name: "f", Image: "test:latest"}
	client := fn.New(fn.WithBuilder(mock.NewBuilder()))
	f, err := client.Init(fConfig)
	if err != nil {
		t.Fatal(err)
	}
	f.Local.Remote = true

	err = f.Write()
	if err != nil {
		t.Fatal(err)
	}

	// Initialise the function directory as a git repo
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}

	// commit the function files
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Commit("init", &git.CommitOptions{
		All:               true,
		AllowEmptyCommits: false,
		Author: &object.Signature{
			Name:  "xyz",
			Email: "xyz@abc.com",
			When:  time.Now(),
		},
		Committer: &object.Signature{
			Name:  "xyz",
			Email: "xyz@abc.com",
			When:  time.Now(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Create a remote and push the function
	remotePath, remoteRm := Mktemp(t)
	defer remoteRm()
	if _, err = git.PlainInit(remotePath, true); err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name:   "origin",
		URLs:   []string{remotePath},
		Mirror: false,
		Fetch:  nil,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Push(&git.PushOptions{
		RemoteName:      "origin",
		RemoteURL:       remotePath,
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatal()
	}

	// Create a new directory to clone the function in
	newRoot, newRm := Mktemp(t)
	defer newRm()

	// Clone the pushed function
	_, err = git.PlainClone(newRoot, false, &git.CloneOptions{
		URL:             remotePath,
		RemoteName:      "origin",
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read the function from the new location
	newFunc, err := fn.NewFunction(newRoot)
	if err != nil {
		t.Fatal(newFunc, err)
	}

	if newFunc.Local.Remote {
		t.Fatal("Remote not supposed to be set")
	}
}
