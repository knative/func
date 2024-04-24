package functions

import (
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
)

// TestMigrated ensures that the .Migrated() method returns whether or not the
// migrations were applied based on its self-reported .SpecVersion member.
func TestMigrated(t *testing.T) {
	vNext := semver.New(LastSpecVersion())
	vNext.BumpMajor()

	tests := []struct {
		name     string
		f        Function
		migrated bool
	}{{
		name:     "no migration stamp",
		f:        Function{},
		migrated: false, // function with no specVersion stamp should be not migrated.
	}, {
		name:     "explicit small specVersion",
		f:        Function{SpecVersion: "0.0.1"},
		migrated: false,
	}, {
		name:     "latest specVersion",
		f:        Function{SpecVersion: LastSpecVersion()},
		migrated: true,
	}, {
		name:     "future specVersion",
		f:        Function{SpecVersion: vNext.String()},
		migrated: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.f.Migrated() != test.migrated {
				t.Errorf("Expected %q.Migrated() to be %t when latest is %q",
					test.f.SpecVersion, test.migrated, LastSpecVersion())
			}
		})
	}
}

// TestMigrate ensures that functions have migrations apply the specVersion
// stamp on instantiation indicating migrations have been applied.
func TestMigrate(t *testing.T) {
	// Load an old function, as it an earlier version it has registered migrations
	// that will need to be applied.
	root := "testdata/migrations/v0.19.0"

	// Instantiate the function with the antiquated structure, which should cause
	// migrations to be applied in order, and result in a function whose version
	// compatibility is equivalent to the latest registered migration.
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.SpecVersion != LastSpecVersion() {
		t.Fatalf("Function was not migrated to %v on instantiation: specVersion is %v",
			LastSpecVersion(), f.SpecVersion)
	}
}

// TestMigrateToCreationStamp ensures that the creation timestamp migration
// introduced for functions 0.19.0 and earlier is applied.
func TestMigrateToCreationStamp(t *testing.T) {
	// Load a function of version 0.19.0, which should have the migration applied
	root := "testdata/migrations/v0.19.0"

	now := time.Now()
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Created.Before(now) {
		t.Fatalf("migration not applied: expected timestamp to be now, got %v.", f.Created)
	}
}

// TestMigrateToBuilderImages ensures that the migration which migrates
// from "builder" and "builders" to "builderImages" is applied.  This results
// in the attributes being removed and no errors on load of the function with
// old schema.
func TestMigrateToBuilderImagesDefault(t *testing.T) {
	// Load a function created prior to the adoption of the builder images map
	// (was created with 'builder' and 'builders' which does not support different
	// builder implementations.
	root := "testdata/migrations/v0.23.0"

	// Without the migration, instantiating the older function would error
	// because its strict unmarshalling would fail parsing the unexpected
	// 'builder' and 'builders' members.
	_, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
}

// TestMigrateToBuilderImagesCustom ensures that the migration to builderImages
// correctly carries forward a customized value for 'builder'.
func TestMigrateToBuilderImagesCustom(t *testing.T) {
	// An early version of a function which includes a customized value for
	// the 'builder'.  This should be correctly carried forward to
	// the namespaced 'builderImages' map as image for the "pack" builder.
	root := "testdata/migrations/v0.23.0-customized"
	expected := "example.com/user/custom-builder" // set in testdata func.yaml

	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(f)
	}
	i, ok := f.Build.BuilderImages["pack"]
	if !ok {
		t.Fatal("migrated function does not include the pack builder images")
	}
	if i != expected {
		t.Fatalf("migrated function expected builder image '%v', got '%v'", expected, i)
	}

}

// TestMigrateToSpecVersion ensures that a func.yaml file with a "version" field
// is migrated to use the field name "specVersion"
func TestMigrateToSpecVersion(t *testing.T) {
	root := "testdata/migrations/v0.25.0"
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.SpecVersion != LastSpecVersion() {
		t.Fatal("migrated function does not include the Migration field")
	}
}

// TestMigrateToSpecs ensures that the migration to the sub-specs format from
// the previous Function structure works
func TestMigrateToSpecs(t *testing.T) {

	root := "testdata/migrations/v0.34.0"
	expectedGit := Git{URL: "http://test-url", Revision: "test revision", ContextDir: "/test/context/dir"}
	expectedNamespace := "test-namespace"
	var expectedEnvs []Env
	var expectedVolumes []Volume

	f, err := NewFunction(root)
	if err != nil {
		t.Error(err)
		t.Fatal(f)
	}

	if f.Build.Git != expectedGit {
		t.Fatalf("migrated Function expected Git '%v', got '%v'", expectedGit, f.Build.Git)
	}

	if f.Deploy.Namespace != expectedNamespace {
		t.Fatalf("migrated Function expected Namespace '%v', got '%v'", expectedNamespace, f.Deploy.Namespace)
	}

	if len(f.Run.Envs) != len(expectedEnvs) {
		t.Fatalf("migrated Function expected Run Envs '%v', got '%v'", len(expectedEnvs), len(f.Run.Envs))
	}

	if len(f.Run.Volumes) != len(expectedVolumes) {
		t.Fatalf("migrated Function expected Run Volumes '%v', got '%v'", len(expectedEnvs), len(f.Run.Envs))
	}

}

// TestMigrateFromInvokeStructure tests that migration from f.Invocation.Format to
// f.Invoke works
func TestMigrateFromInvokeStructure(t *testing.T) {
	root0 := "testdata/migrations/v0.35.0"
	expectedInvoke := "" // empty because http is default and not written in yaml file

	f0, err := NewFunction(root0)
	if err != nil {
		t.Error(err)
		t.Fatal(f0)
	}
	if f0.Invoke != expectedInvoke {
		t.Fatalf("migrated Function expected Invoke '%v', got '%v'", expectedInvoke, f0.Invoke)
	}

	root1 := "testdata/migrations/v0.35.0-nondefault"
	expectedInvoke = "cloudevent"
	f1, err := NewFunction(root1)
	if err != nil {
		t.Error(err)
		t.Fatal(f1)
	}
	if f1.Invoke != expectedInvoke {
		t.Fatalf("migrated Function expected Invoke '%v', got '%v'", expectedInvoke, f0.Invoke)
	}
}
