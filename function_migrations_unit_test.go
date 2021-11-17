package function

import (
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
)

// TestMigrated ensures that the .Migrated() method returns whether or not the
// migrations were applied based on its self-reported .Version member.
func TestMigrated(t *testing.T) {
	// A Function with no migration stamp
	f := Function{}
	if f.Migrated() {
		t.Fatal("function with no version stamp should be not migrated.")
	}

	// A Function with a migration stamp that is explicitly less than the
	// latest known.
	f = Function{Version: "0.0.1"}
	if f.Migrated() {
		t.Fatalf("function with version %v when latest is %v should be !migrated",
			f.Version, latestMigrationVersion())
	}

	// A Function with a version stamp equivalent to the latest is up-to-date
	// and should be considered migrated.
	f = Function{Version: latestMigrationVersion()}
	if !f.Migrated() {
		t.Fatalf("function version %v should me considered migrated (latest is %v)",
			f.Version, latestMigrationVersion())
	}

	// A Function with a version stamp beyond what is recognized by the current
	// codebase is considered fully migrated, for purposes of this version of func
	v0 := semver.New(latestMigrationVersion())
	v0.BumpMajor()
	f = Function{Version: v0.String()}
	if !f.Migrated() {
		t.Fatalf("Function with version %v should be considered migrated when latest known by this codebase is %v", f.Version, latestMigrationVersion())
	}
}

// TestMigrate ensures that Functions have migrations apply the version
// stamp on instantiation indicating migrations have been applied.
func TestMigrate(t *testing.T) {
	// Load an old Function, as it an earlier version it has registered migrations
	// that will need to be applied.
	root := "testdata/migrations/v0.19.0"

	// Instantiate the Function with the antiquated structure, which should cause
	// migrations to be applied in order, and result in a function whose version
	// compatibility is equivalent to the latest registered migration.
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != latestMigrationVersion() {
		t.Fatalf("Function was not migrated to %v on instantiation: version is %v",
			latestMigrationVersion(), f.Version)
	}
}

// TestMigrateToCreationStamp ensures that the creation timestamp migration
// introduced for functions 0.19.0 and earlier is applied.
func TestMigrateToCreationStamp(t *testing.T) {
	// Load a Function of version 0.19.0, which should have the migration applied
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

func latestMigrationVersion() string {
	return migrations[len(migrations)-1].version
}
