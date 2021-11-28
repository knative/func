package function

import (
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
)

// TestMigrated ensures that the .Migrated() method returns whether or not the
// migrations were applied based on its self-reported .Version member.
func TestMigrated(t *testing.T) {
	vNext := semver.New(latestMigrationVersion())
	vNext.BumpMajor()

	tests := []struct {
		name     string
		f        Function
		migrated bool
	}{{
		name:     "no migration stamp",
		f:        Function{},
		migrated: false, // function with no version stamp should be not migrated.
	}, {
		name:     "explicit small version",
		f:        Function{Version: "0.0.1"},
		migrated: false,
	}, {
		name:     "latest version",
		f:        Function{Version: latestMigrationVersion()},
		migrated: true,
	}, {
		name:     "future version",
		f:        Function{Version: vNext.String()},
		migrated: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.f.Migrated() != test.migrated {
				t.Errorf("Expected %q.Migrated() to be %t when latest is %q",
					test.f.Version, test.migrated, latestMigrationVersion())
			}
		})
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
