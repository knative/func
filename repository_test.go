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
