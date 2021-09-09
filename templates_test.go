// +build !integration

package function_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// TestTemplatesList ensures that all templates are listed taking into account
// both internal and extensible (prefixed) repositories.
func TestTemplatesList(t *testing.T) {
	// A client which specifies a location of exensible repositoreis on disk
	// will list all builtin plus exensible
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// list templates for the "go" runtime
	templates, err := client.Templates().List("go")
	if err != nil {
		t.Fatal(err)
	}

	// Note that this list will change as the customProvider
	// and builtin templates are shared.  THis could be mitigated
	// by creating a custom repository path for just this test, if
	// that becomes a hassle.
	expected := []string{
		"events",
		"http",
		"customProvider/customTemplate",
		"repositoryTests/custom",
	}

	if !reflect.DeepEqual(templates, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", templates)
		t.Fatal("Expected templates list not received.")
	}
}

// TestTemplatesGet ensures that a template's metadata object can
// be retrieved by full name (full name prefix optional for embedded).
func TestTemplatesGet(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// Check embedded

	embedded, err := client.Templates().Get("go", "http")
	if err != nil {
		t.Fatal(err)
	}

	expected := fn.Template{
		Runtime:    "go",
		Repository: "default",
		Name:       "http",
	}

	if !reflect.DeepEqual(embedded, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", embedded)
		t.Fatal("Template from embedded repo not as expected.")
	}

	// Check extended

	extended, err := client.Templates().Get("go", "customProvider/customTemplate")
	if err != nil {
		t.Fatal(err)
	}

	expected = fn.Template{
		Runtime:    "go",
		Repository: "customProvider",
		Name:       "customTemplate",
	}

	if !reflect.DeepEqual(extended, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", extended)
		t.Fatal("Template from extended repo not as expected.")
	}
}

// TestTemplateEmbedded ensures that embedded templates are copied on write.
func TestTemplateEmbedded(t *testing.T) {
	// create test directory
	root := "testdata/testTemplateEmbedded"
	defer using(t, root)()

	// Client whose internal (builtin default) templates will be used.
	client := fn.New(fn.WithRegistry(TestRegistry))

	// write out a template
	err := client.Create(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "tpla",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "rtAtplA.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplateCustom ensures that a template from a filesystem source
// (ie. custom provider on disk) can be specified as the source for a
// template.
func TestTemplateCustom(t *testing.T) {
	// Create test directory
	root := "testdata/testTemplateCustom"
	defer using(t, root)()

	// CLient which uses custom repositories
	// in form [provider]/[template], on disk the template is
	// at: testdata/repositories/[provider]/[runtime]/[template]
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositories("testdata/repositories"))

	// Create a function specifying a template from
	// the custom provider's directory in the on-disk template repo.
	err := client.Create(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "customProvider/tpla",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "customtpl.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplateRemote ensures that a Git template repository provided via URI
// can be specificed.
func TestTemplateRemote(t *testing.T) {
	// Create test directory
	root := "testdata/testTemplateRemote"
	defer using(t, root)()

	// The difference between HTTP vs File protocol is internal to the
	// go-git library which implements the template writer.  As such
	// providing a local file URI is conceptually sufficient to test
	// our usage, though in practice HTTP is expected to be the norm.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "testdata", "repository.git")
	url := fmt.Sprintf(`file://%s`, path)

	t.Logf("cloning: %v", url)

	// Create a client which explicitly specifies the Git repo at URL
	// rather than relying on the default internally builtin template repo
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepository(url),
	)

	// Create a default function, which should override builtin and use
	// that from the specified url (git repo)
	err = client.Create(fn.Function{
		Root:     root,
		Runtime:  "go",
		Template: "remote",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert the sample file from the git repo was written
	_, err = os.Stat(filepath.Join(root, "remote-test"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplateDefault ensures that the expected default template
// is used when none specified.
func TestTemplateDefault(t *testing.T) {
	// create test directory
	root := "testdata/testTemplateDefault"
	defer using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// The runtime is specified, and explicitly includes a
	// file for the default template of fn.DefaultTemplate
	err := client.Create(fn.Function{Root: root, Runtime: TestRuntime})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "rtAtplDefault.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplateInvalidErrors ensures that specifying unrecgognized
// runtime/template errors
func TestTemplateInvalidErrors(t *testing.T) {
	// create test directory
	root := "testdata/testTemplateInvalidErrors"
	defer using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Error will be type-checked.
	var err error

	// Test for error writing an invalid runtime
	err = client.Create(fn.Function{
		Root:    root,
		Runtime: "invalid",
	})
	if !errors.Is(err, fn.ErrRuntimeNotFound) {
		t.Fatalf("Expected ErrRuntimeNotFound, got %v", err)
	}

	// Test for error writing an invalid template
	err = client.Create(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "invalid",
	})
	if !errors.Is(err, fn.ErrTemplateNotFound) {
		t.Fatalf("Expected ErrTemplateNotFound, got %v", err)
	}
}

// TestTemplateModeEmbedded ensures that templates written from the embedded
// templates retain their mode.
func TestTemplateModeEmbedded(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
		// not applicable
	}

	// set up test directory
	root := "testdata/testTemplateModeEmbedded"
	defer using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Write the embedded template that contains an executable
	err := client.Create(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "tplb",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify file mode was preserved
	file, err := os.Stat(filepath.Join(root, "executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode() != os.FileMode(0755) {
		t.Fatalf("The embedded executable's mode should be 0755 but was %v", file.Mode())
	}
}

// TestTemplateModeCustom ensures that templates written from custom templates
// retain their mode.
func TestTemplateModeCustom(t *testing.T) {
	if runtime.GOOS == "windows" {
		return // not applicable
	}

	// test directories
	root := "testdata/testTemplateModeCustom"
	defer using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositories("testdata/repositories"))

	// Write executable from custom repo
	err := client.Create(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "customProvider/tplb",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify custom file mode was preserved.
	file, err := os.Stat(filepath.Join(root, "executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode() != os.FileMode(0755) {
		t.Fatalf("The custom executable file's mode should be 0755 but was %v", file.Mode())
	}
}

// TestTemplateModeRemote ensures that templates written from remote templates
// retain their mode.
func TestTemplateModeRemote(t *testing.T) {
	if runtime.GOOS == "windows" {
		return // not applicable
	}

	// test directories
	root := "testdata/testTemplateModeRemote"
	defer using(t, root)()

	// Clone a repository from a local file path
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "testdata", "repository.git")
	url := fmt.Sprintf(`file://%s`, path)

	t.Logf("cloning: %v", url)

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepository(url))

	// Write executable from custom repo
	err = client.Create(fn.Function{
		Root:     root,
		Runtime:  "node",
		Template: "remote",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify directory file mode was preserved
	file, err := os.Stat(filepath.Join(root, "test"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode() != os.ModeDir|0755 {
		t.Fatalf("The remote repositry directory mode should be 0755 but was %#o", file.Mode())
	}

	// Verify remote executible file mode was preserved.
	file, err = os.Stat(filepath.Join(root, "test", "executable.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode() != os.FileMode(0755) {
		t.Fatalf("The remote executable's mode should be 0755 but was %v", file.Mode())
	}
}

// TODO: test typed errors for custom and remote (embedded checked)
