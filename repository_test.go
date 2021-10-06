package function_test

import (
	"reflect"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// TestRepositoryGetTemplateEmbedded ensures that repositories make templates
// avaialble via the Get accessor from the embedded repository.
func TestRepositoryGetTemplateEmbedded(t *testing.T) {
	client := fn.New()

	// unless overridden using remote single-repo option the default repo
	// is the embedded
	repo, err := client.Repositories().Get(fn.DefaultRepositoryName)
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
// avaialble via the Get accessor from a custom-installed (on disk) repo.
func TestRepositoryGetTemplateCustom(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	repo, err := client.Repositories().Get("customTemplateRepo")
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.Template("customRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// degenerate case: API of a custom repository should return what it was
	// expressly asked for at minimum (known good request)
	if template.Name != "customTemplate" {
		t.Logf("expected custom repo to yield a template named 'customTemplate', got '%v'", template.Name)
	}
}

// TestManifestedRepositoryGetTemplate ensures that repositories make templates
// available via the Get accessor when a manifest.yaml is included which
// defines templates to be located in a custom location (such as with language
// packs which will likely place them not in root but in ./templates)
func TestManifestedRepositoryGetTemplate(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// The repo ./testdata/repositories/customLanguagePackRepo includes a
	// manifest.yaml which defines templates as existing in the ./templates
	// directory within the repo.
	repo, err := client.Repositories().Get("customLanguagePackRepo")
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.Template("customRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// degenerate case: API of a custom repository should return what it was
	// expressly asked for at minimum (known good request)
	if template.Name != "customTemplate" {
		t.Logf("expected custom language pack repo to yield a template named 'customTemplate', got '%v'", template.Name)
	}
}

// TestManifestedRepositoryInheritance ensures that repositories which define
// a manifest properly inherit values defined at the repo level, runtime level
// and template level.  The tests check for one attribute of both embedded
// structures: HealthEndpoint's Readiness and BuildConfig's Buildpacks
// should apply to all shared attributes u
func TestManifestedRepositoryInheritance(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// The repo ./testdata/repositories/customLanguagePack includes a manifest
	// which defines custom readiness and liveness endpoints.
	// The runtime "manifestedRuntime" includes a manifest which sets these
	// for all templates within, and the template "manifestedTemplate" sets
	// them explicitly for itself.
	repo, err := client.Repositories().Get("customLanguagePackRepo")
	if err != nil {
		t.Fatal(err)
	}

	// Template A:  from a path containing no settings other than the repo root.
	// Should have a readiness and liveness equivalent to that defined in
	// [repo]/manifest.yaml
	tA, err := repo.Template("customRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// Template B: from a path containing runtime-wide settings, but no
	// template-level settings.
	tB, err := repo.Template("manifestedRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// Template C: from a runtime with a manifest which sets endpoints, and
	// itself includes a manifest which explicitly sets.
	tC, err := repo.Template("manifestedRuntime", "manifestedTemplate")
	if err != nil {
		t.Fatal(err)
	}

	// Assert Template A reflects repo-level settings
	if tA.Readiness != "/repoReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(tA.Buildpacks, []string{"repoBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}

	// Assert Template B reflects runtime-level settings
	if tB.Readiness != "/runtimeReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(tB.Buildpacks, []string{"runtimeBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}

	// Assert Template C reflects template-level settings
	if tC.Readiness != "/templateReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(tC.Buildpacks, []string{"templateBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}

}
