package function_test

import (
	"reflect"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// TestRepositoryGetTemplateDefault ensures that repositories make templates
// avaialble via the Get accessor which given name and runtime.
func TestRepositoryGetTemplateDefault(t *testing.T) {
	client := fn.New()

	repo, err := client.Repositories().Get(fn.DefaultRepository)
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.GetTemplate("go", "http")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Template{
		Runtime:    "go",
		Repository: fn.DefaultRepository,
		Name:       "http",
	}
	if !reflect.DeepEqual(template, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", template)
		t.Fatal("Default template not as expected")
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
	template, err := repo.GetTemplate("go", "custom")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Template{
		Runtime:    "go",
		Repository: "repositoryTests",
		Name:       "custom",
	}
	if !reflect.DeepEqual(template, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", template)
		t.Fatal("Custom template not as expected")
	}

}

// TestRepositoryGetRuntimeDefault ensures that repositories make runtimes
// avaialble via the Get accessor with given name.
func TestRepositoryGetRuntimeDefault(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	repo, err := client.Repositories().Get("repositoryTests")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := repo.GetRuntime("go")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Runtime{
		Name: "go",
		Path: "go",
		Templates: fn.FunctionTemplates{
			{
				Name: "custom",
				Path: "custom",
			},
		},
	}
	if runtime.Name != expected.Name {
		t.Fatalf("Expected: %s\nGot: %s", expected.Name, runtime.Name)
	}
	if runtime.Path != expected.Path {
		t.Fatalf("Expected: %s\nGot: %s", expected.Path, runtime.Path)
	}
	if !reflect.DeepEqual(runtime.Templates, expected.Templates) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", runtime)
		t.Fatal("Custom go runtime not as expected")
	}
}

// TestRepositoryGetRuntimeDefault ensures that repositories make runtimes
// avaialble via the Get accessor with given name.
func TestRepositoryGetRuntimeCustom(t *testing.T) {
	client := fn.New()

	repo, err := client.Repositories().Get(fn.DefaultRepository)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := repo.GetRuntime("go")
	if err != nil {
		t.Fatal(err)
	}
	expected := fn.Runtime{
		Name: "go",
		Path: "go",
		Templates: fn.FunctionTemplates{
			{
				Name: "events",
				Path: "events",
			},
			{
				Name: "http",
				Path: "http",
			},
		},
	}
	if runtime.Name != expected.Name {
		t.Fatalf("Expected: %s\nGot: %s", expected.Name, runtime.Name)
	}
	if runtime.Path != expected.Path {
		t.Fatalf("Expected: %s\nGot: %s", expected.Path, runtime.Path)
	}
	if !reflect.DeepEqual(runtime.Templates, expected.Templates) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", runtime)
		t.Fatal("Default go runtime not as expected")
	}
}
