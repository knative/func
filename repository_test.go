package function_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// TestRepositoryTemplatesPath ensures that repositories can specify
// an alternate location for templates using a manifest.
func TestRepositoryTemplatesPath(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// The repo ./testdata/repositories/customLanguagePackRepo includes a
	// manifest.yaml which defines templates as existing in the ./templates
	// directory within the repo.
	repo, err := client.Repositories().Get("customLanguagePackRepo")
	if err != nil {
		t.Fatal(err)
	}
	template, err := repo.Template(context.TODO(), "customRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// degenerate case: API of a custom repository should return what it was
	// expressly asked for at minimum (known good request)
	if template.Name() != "customTemplate" {
		t.Logf("expected custom language pack repo to yield a template named 'customTemplate', got '%v'", template.Name())
	}
}

// TestRepositoryInheritance ensures that repositories which define a manifest
// properly inherit values defined at the repo level, runtime level
// and template level.  The tests check for both embedded structures:
// HealthEndpoints BuildConfig.
func TestRepositoryInheritance(t *testing.T) {
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
	tA, err := repo.Template(context.TODO(), "customRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// Template B: from a path containing runtime-wide settings, but no
	// template-level settings.
	tB, err := repo.Template(context.TODO(), "manifestedRuntime", "customTemplate")
	if err != nil {
		t.Fatal(err)
	}
	// Template C: from a runtime with a manifest which sets endpoints, and
	// itself includes a manifest which explicitly sets.
	tC, err := repo.Template(context.TODO(), "manifestedRuntime", "manifestedTemplate")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()

	fnADir := filepath.Join(tmpDir, "fn-a")
	err = tA.Write(context.TODO(), "fn-a", fnADir)
	if err != nil {
		t.Fatal(err)
	}
	fnBDir := filepath.Join(tmpDir, "fn-b")
	err = tB.Write(context.TODO(), "fn-b", fnBDir)
	if err != nil {
		t.Fatal(err)
	}
	fnCDir := filepath.Join(tmpDir, "fn-c")
	err = tC.Write(context.TODO(), "fn-c", fnCDir)
	if err != nil {
		t.Fatal(err)
	}

	fA, _ := fn.NewFunction(fnADir)
	fB, _ := fn.NewFunction(fnBDir)
	fC, _ := fn.NewFunction(fnCDir)

	// Assert Template A reflects repo-level settings
	if fA.HealthEndpoints.Readiness != "/repoReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(fA.Buildpacks, []string{"repoBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}

	// Assert Template B reflects runtime-level settings
	if fB.HealthEndpoints.Readiness != "/runtimeReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(fB.Buildpacks, []string{"runtimeBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}

	// Assert Template C reflects template-level settings
	if fC.HealthEndpoints.Readiness != "/templateReadiness" {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
	if !reflect.DeepEqual(fC.Buildpacks, []string{"templateBuildpack"}) {
		t.Fatalf("Repository-level HealthEndpoint not loaded to template")
	}
}
