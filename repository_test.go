package function_test

import (
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// TestRepositoryGetTemplateDefault ensures that repositories make templates
// avaialble via the Get accessor when given name and runtime.
func TestRepositoryGetTemplateDefault(t *testing.T) {
	client := fn.New()

	repo, err := client.Repositories().Get(fn.DefaultRepository)
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.Template("go", "http")
	if err != nil {
		t.Fatal(err)
	}
	// degenerate case: API of the default repository should return what it was
	// expressly asked for at minimum (known good request)
	if template.Name != "http" {
		t.Logf("expected default repo to yield a template named 'http', got '%v'", template.Name)
	}
}

// TestRepositoryGetTemplateCustom ensures that repositories make templates
// avaialble via the Get accessor with given name and runtime.
func TestRepositoryGetTemplateCustom(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	repo, err := client.Repositories().Get("repositoryTests")
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.Template("go", "custom")
	if err != nil {
		t.Fatal(err)
	}
	// degenerate case: API of a custom repository should return what it was
	// expressly asked for at minimum (known good request)
	if template.Name != "custom" {
		t.Logf("expected custom repo to yield a template named 'http', got '%v'", template.Name)
	}

}

// TestRepositoryGetRuntimeDefault ensures that repositories make runtimes
// available via the Get accessor with given name.
func TestRepositoryGetRuntimeDefault(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	repo, err := client.Repositories().Get("repositoryTests")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := repo.Runtime("go")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Runtime{
		Name: "go",
		Templates: []fn.Template{
			{
				Name: "custom",
			},
		},
	}
	if runtime.Name != expected.Name {
		t.Fatalf("Expected runtime name '%s', got '%s'",
			expected.Name, runtime.Name)
	}
	if len(runtime.Templates) != len(expected.Templates) {
		t.Fatalf("Expected test runtime to have %v template, got %v",
			len(expected.Templates), len(runtime.Templates))
	}
	if runtime.Templates[0].Name != expected.Templates[0].Name {
		t.Fatalf("Expected first returned template's name to be '%v', got '%v'",
			expected.Templates[0].Name, runtime.Templates[0].Name)
	}
}

// TestRepositoryGetRuntimeDefault ensures that repositories make runtimes
// available via the Get accessor with given name.
func TestRepositoryGetRuntimeCustom(t *testing.T) {
	client := fn.New()

	repo, err := client.Repositories().Get(fn.DefaultRepository)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := repo.Runtime("go")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Runtime{
		Name: "go",
		Templates: []fn.Template{
			{
				Name: "events",
			},
			{
				Name: "http",
			},
		},
	}
	if runtime.Name != expected.Name {
		t.Fatalf("Expected runtime name '%s', got '%s'",
			expected.Name, runtime.Name)
	}
	if len(runtime.Templates) != len(expected.Templates) {
		t.Fatalf("Expected test runtime to have %v template, got %v",
			len(expected.Templates), len(runtime.Templates))
	}
	if runtime.Templates[0].Name != expected.Templates[0].Name {
		t.Fatalf("Expected first returned template's name to be '%v', got '%v'",
			expected.Templates[0].Name, runtime.Templates[0].Name)
	}
}
