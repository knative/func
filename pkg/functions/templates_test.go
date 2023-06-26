//go:build !integration
// +build !integration

package functions_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestTemplates_List ensures that all templates are listed taking into account
// both internal and extensible (prefixed) repositories.
func TestTemplates_List(t *testing.T) {
	// A client which specifies a location of exensible repositoreis on disk
	// will list all builtin plus exensible
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	// list templates for the "go" runtime
	templates, err := client.Templates().List("go")
	if err != nil {
		t.Fatal(err)
	}

	// Note that this list will change as the customTemplateRepo
	// and builtin templates are shared.  THis could be mitigated
	// by creating a custom repository path for just this test, if
	// that becomes a hassle.
	expected := []string{
		"cloudevents",
		"http",
		"customTemplateRepo/customTemplate",
	}

	if diff := cmp.Diff(expected, templates); diff != "" {
		t.Error("Unexpected templates (-want, +got):", diff)
	}
}

// TestTemplates_List_ExtendedNotFound ensures that an error is not returned
// when retrieving the list of templates for a runtime that does not exist
// in an extended repository, but does in the default.
func TestTemplates_List_ExtendedNotFound(t *testing.T) {
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	// list templates for the "python" runtime -
	// not supplied by the extended repos
	templates, err := client.Templates().List("python")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"cloudevents",
		"flask",
		"http",
		"wsgi",
	}

	if diff := cmp.Diff(expected, templates); diff != "" {
		t.Error("Unexpected templates (-want, +got):", diff)
	}
}

// TestTemplates_Get ensures that a template's metadata object can
// be retrieved by full name (full name prefix optional for embedded).
func TestTemplates_Get(t *testing.T) {
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	// Check embedded
	embedded, err := client.Templates().Get("go", "http")
	if err != nil {
		t.Fatal(err)
	}

	if embedded.Runtime() != "go" || embedded.Repository() != "default" || embedded.Name() != "http" {
		t.Logf("Expected template from embedded to have runtime 'go' repo 'default' name 'http', got '%v', '%v', '%v',",
			embedded.Runtime(), embedded.Repository(), embedded.Name())
	}

	// Check extended
	extended, err := client.Templates().Get("go", "customTemplateRepo/customTemplate")
	if err != nil {
		t.Fatal(err)
	}

	if embedded.Runtime() != "go" || embedded.Repository() != "default" || embedded.Name() != "http" {
		t.Logf("Expected template from extended repo to have runtime 'go' repo 'customTemplateRepo' name 'customTemplate', got '%v', '%v', '%v',",
			extended.Runtime(), extended.Repository(), extended.Name())
	}
}

// TestTemplates_Embedded ensures that embedded templates are copied on write.
func TestTemplates_Embedded(t *testing.T) {
	// create test directory
	root := "testdata/testTemplatesEmbedded"
	defer Using(t, root)()

	// Client whose internal (builtin default) templates will be used.
	client := fn.New(fn.WithRegistry(TestRegistry))

	// write out a template
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "http",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "handle.go"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplates_Custom ensures that a template from a filesystem source
// (ie. custom provider on disk) can be specified as the source for a
// template.
func TestTemplates_Custom(t *testing.T) {
	// Create test directory
	root := "testdata/testTemplatesCustom"
	defer Using(t, root)()

	// CLient which uses custom repositories
	// in form [provider]/[template], on disk the template is
	// at: testdata/repositories/[provider]/[runtime]/[template]
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// Create a function specifying a template from
	// the custom provider's directory in the on-disk template repo.
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "customRuntime",
		Template: "customTemplateRepo/customTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "custom.impl"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplates_Remote ensures that a Git template repository provided via URI
// can be specificed on creation of client, with subsequent calls to Create
// using this remote by default.
func TestTemplates_Remote(t *testing.T) {
	var err error

	root := "testdata/testTemplatesRemote"
	defer Using(t, root)()

	url := ServeRepo(RepositoriesTestRepo, t)

	// Create a client which explicitly specifies the Git repo at URL
	// rather than relying on the default internally builtin template repo
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepository(url))

	// Create a default function, which should override builtin and use
	// that from the specified url (git repo)
	_, err = client.Init(fn.Function{
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

// TestTemplates_Default ensures that the expected default template
// is used when none specified.
func TestTemplates_Default(t *testing.T) {
	// create test directory
	root := "testdata/testTemplates_Default"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// The runtime is specified, and explicitly includes a
	// file for the default template of fn.DefaultTemplate
	_, err := client.Init(fn.Function{Root: root, Runtime: TestRuntime})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "handle.go"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestTemplates_InvalidErrors ensures that specifying unrecgognized
// runtime/template errors
func TestTemplates_InvalidErrors(t *testing.T) {
	// create test directory
	root := "testdata/testTemplates_InvalidErrors"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Error will be type-checked.
	var err error

	// Test for error writing an invalid runtime
	_, err = client.Init(fn.Function{
		Root:    root,
		Runtime: "invalid",
	})
	if !errors.Is(err, fn.ErrRuntimeNotFound) {
		t.Fatalf("Expected ErrRuntimeNotFound, got %v", err)
	}
	os.Remove(filepath.Join(root, ".gitignore"))

	// Test for error writing an invalid template
	_, err = client.Init(fn.Function{
		Root:     root,
		Runtime:  TestRuntime,
		Template: "invalid",
	})
	if !errors.Is(err, fn.ErrTemplateNotFound) {
		t.Fatalf("Expected ErrTemplateNotFound, got %v", err)
	}
}

// TestTemplates_ModeEmbedded ensures that templates written from the embedded
// templates retain their mode.
func TestTemplates_ModeEmbedded(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
		// not applicable
	}

	// set up test directory
	root := "testdata/testTemplatesModeEmbedded"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Write the embedded template that contains a file which
	// needs to be executable (only such is mvnw in quarkus)
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "quarkus",
		Template: "http",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify file mode was preserved
	file, err := os.Stat(filepath.Join(root, "mvnw"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode() != os.FileMode(0755) {
		t.Fatalf("The embedded executable's mode should be 0755 but was %v", file.Mode())
	}
}

// TestTemplates_ModeCustom ensures that templates written from custom templates
// retain their mode.
func TestTemplates_ModeCustom(t *testing.T) {
	if runtime.GOOS == "windows" {
		return // not applicable
	}

	// test directories
	root := "testdata/testTemplates_ModeCustom"
	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// Write executable from custom repo
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "test",
		Template: "customTemplateRepo/tplb",
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

// TestTemplates_ModeRemote ensures that templates written from remote templates
// retain their mode.
func TestTemplates_ModeRemote(t *testing.T) {
	var err error

	if runtime.GOOS == "windows" {
		return // not applicable
	}

	// test directories
	root := "testdata/testTemplates_ModeRemote"
	defer Using(t, root)()

	url := ServeRepo(RepositoriesTestRepo, t)

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepository(url))

	// Write executable from custom repo
	_, err = client.Init(fn.Function{
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

// TestTemplates_RuntimeManifestBuildEnvs ensures that BuildEnvs specified in a
// runtimes's manifest are included in the final function.
func TestTemplates_RuntimeManifestBuildEnvs(t *testing.T) {
	// create test directory
	root := "testdata/testTemplatesRuntimeManifestBuildEnvs"
	defer Using(t, root)()

	// Client whose internal templates will be used.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// write out a template
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "manifestedRuntime",
		Template: "customLanguagePackRepo/customTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	testVariableName := "TEST_RUNTIME_VARIABLE"
	testVariableValue := "test-runtime"

	envs := []fn.Env{
		{
			Name:  &testVariableName,
			Value: &testVariableValue,
		},
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(fn.Envs(envs), f.Build.BuildEnvs); diff != "" {
		t.Fatalf("Unexpected difference between runtime's manifest.yaml envs and function BuildEnvs (-want, +got): %v", diff)
	}
}

// TestTemplates_ManifestBuildEnvs ensures that BuildEnvs specified in a
// template's manifest are included in the final function.
func TestTemplates_ManifestBuildEnvs(t *testing.T) {
	// create test directory
	root := "testdata/testTemplatesManifestBuildEnvs"
	defer Using(t, root)()

	// Client whose internal templates will be used.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// write out a template
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "manifestedRuntime",
		Template: "customLanguagePackRepo/manifestedTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	testVariableName := "TEST_TEMPLATE_VARIABLE"
	testVariableValue := "test-template"

	envs := []fn.Env{
		{
			Name:  &testVariableName,
			Value: &testVariableValue,
		},
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(fn.Envs(envs), f.Build.BuildEnvs); diff != "" {
		t.Fatalf("Unexpected difference between template's manifest.yaml envs and function BuildEnvs (-want, +got): %v", diff)
	}
}

// TestTemplates_RepositoryManifestBuildEnvs ensures that BuildEnvs specified in a
// repository's manifest are included in the final function.
func TestTemplates_RepositoryManifestBuildEnvs(t *testing.T) {
	// create test directory
	root := "testdata/testRepositoryManifestBuildEnvs"
	defer Using(t, root)()

	// Client whose internal templates will be used.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// write out a template
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "customRuntime",
		Template: "customLanguagePackRepo/customTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert file exists as expected
	_, err = os.Stat(filepath.Join(root, "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	testVariableName := "TEST_REPO_VARIABLE"
	testVariableValue := "test-repo"

	envs := []fn.Env{
		{
			Name:  &testVariableName,
			Value: &testVariableValue,
		},
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(fn.Envs(envs), f.Build.BuildEnvs); diff != "" {
		t.Fatalf("Unexpected difference between repository's manifest.yaml envs and function BuildEnvs (-want, +got): %v", diff)
	}
}

// TestTemplates_ManifestInvocationHints ensures that invocation hints
// from a template's manifest are included in the final function.
func TestTemplates_ManifestInvocationHints(t *testing.T) {
	root := "testdata/testTemplatesManifestInvocationHints"
	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	f, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "manifestedRuntime",
		Template: "customLanguagePackRepo/manifestedTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	if f.Invoke != "format" {
		t.Fatalf("expected invoke format 'format', got '%v'", f.Invoke)
	}
}

// TestTemplates_ManifestRemoved ensures that the manifest is not left in
// the resultant function after write.
func TestTemplates_ManifestRemoved(t *testing.T) {
	// create test directory
	root := "testdata/testTemplateManifestRemoved"
	defer Using(t, root)()

	// Client whose internal templates will be used.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// write out a template
	_, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "manifestedRuntime",
		Template: "customLanguagePackRepo/manifestedTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert func.yaml exists as expected
	_, err = os.Stat(filepath.Join(root, "func.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Assert manifest.yaml does not
	_, err = os.Stat(filepath.Join(root, "manifest.yaml"))
	if err == nil {
		t.Fatal("manifest.yaml should not exist after write")
	}

}

// TestTemplates_InvocationDefault ensures that creating a function which
// does not define an invocation hint defaults to empty string (since 0.35.0
// default value is omitted from func.yaml file for Invoke)
func TestTemplates_InvocationDefault(t *testing.T) {
	expectedInvoke := ""
	root := "testdata/testTemplatesInvocationDefault"
	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRepositoriesPath("testdata/repositories"))

	// The customTemplateRepo explicitly does not
	// include manifests as it exemplifies an entirely default template repo.
	f, err := client.Init(fn.Function{
		Root:     root,
		Runtime:  "customRuntime",
		Template: "customTemplateRepo/customTemplate",
	})
	if err != nil {
		t.Fatal(err)
	}

	if f.Invoke != expectedInvoke {
		t.Fatalf("expected '%v' invoke format.  Got '%v'", expectedInvoke, f.Invoke)
	}
}
