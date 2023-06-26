//go:build !integration
// +build !integration

package functions_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	fn "knative.dev/func/pkg/functions"
)

// TestRepository_TemplatesPath ensures that repositories can specify
// an alternate location for templates using a manifest.
func TestRepository_TemplatesPath(t *testing.T) {
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

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
	if template.Name() != "customTemplate" {
		t.Logf("expected custom language pack repo to yield a template named 'customTemplate', got '%v'", template.Name())
	}
}

// TestRepository_Inheritance ensures that repositories which define a manifest
// properly inherit values defined at the repo level, runtime level
// and template level.  The tests check for both embedded structures:
// HealthEndpoints BuildConfig.
func TestRepository_Inheritance(t *testing.T) {
	var err error
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	repo, err := client.Repositories().Get("customLanguagePackRepo")
	if err != nil {
		t.Fatal(err)
	}

	// Template A:  from a path containing no settings other than the repo root.
	// Should have a readiness and liveness equivalent to that defined in
	// [repo]/manifest.yaml
	fA := fn.Function{
		Name: "fn-a",
		Root: t.TempDir(),
	}
	tA, err := repo.Template("customRuntime", "customTemplate")
	if err != nil {
		t.Error(err)
	}
	err = tA.Write(context.Background(), &fA)
	if err != nil {
		t.Error(err)
	}

	// Template B: from a path containing runtime-wide settings, but no
	// template-level settings.
	fB := fn.Function{
		Name: "fn-b",
		Root: t.TempDir(),
	}
	tB, err := repo.Template("manifestedRuntime", "customTemplate")
	if err != nil {
		t.Error(err)
	}
	err = tB.Write(context.Background(), &fB)
	if err != nil {
		t.Error(err)
	}

	// Template C: from a runtime with a manifest which sets endpoints, and
	// itself includes a manifest which explicitly sets.
	fC := fn.Function{
		Name: "fn-c",
		Root: t.TempDir(),
	}
	tC, err := repo.Template("manifestedRuntime", "manifestedTemplate")
	if err != nil {
		t.Error(err)
	}
	err = tC.Write(context.Background(), &fC)
	if err != nil {
		t.Error(err)
	}

	// Assert Template A reflects repo-level settings
	if fA.Deploy.HealthEndpoints.Readiness != "/repoReadiness" {
		t.Errorf("Repository-level HealthEndpoint not loaded to template, got %q", fA.Deploy.HealthEndpoints.Readiness)
	}
	if diff := cmp.Diff([]string{"repoBuildpack"}, fA.Build.Buildpacks); diff != "" {
		t.Errorf("Repository-level Buildpack differs (-want, +got): %s", diff)
	}

	// Assert Template B reflects runtime-level settings
	if fB.Deploy.HealthEndpoints.Readiness != "/runtimeReadiness" {
		t.Errorf("Runtime-level HealthEndpoint not loaded to template, got %q", fB.Deploy.HealthEndpoints.Readiness)
	}
	if diff := cmp.Diff([]string{"runtimeBuildpack"}, fB.Build.Buildpacks); diff != "" {
		t.Errorf("Runtime-level Buildpack differs (-want, +got): %s", diff)
	}

	envVarName := "TEST_RUNTIME_VARIABLE"
	envVarValue := "test-runtime"
	envs := []fn.Env{
		{
			Name:  &envVarName,
			Value: &envVarValue,
		},
	}

	if diff := cmp.Diff(fn.Envs(envs), fB.Build.BuildEnvs); diff != "" {
		t.Fatalf("Unexpected difference between repository's manifest.yaml envs and function BuildEnvs (-want, +got): %v", diff)
	}

	// Assert Template C reflects template-level settings
	if fC.Deploy.HealthEndpoints.Readiness != "/templateReadiness" {
		t.Fatalf("Template-level HealthEndpoint not loaded to template, got %q", fC.Deploy.HealthEndpoints.Readiness)
	}
	if diff := cmp.Diff([]string{"templateBuildpack"}, fC.Build.Buildpacks); diff != "" {
		t.Fatalf("Template-level Buildpack differs (-want, +got): %s", diff)
	}
}
