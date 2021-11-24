package function

import (
	"time"

	"github.com/coreos/go-semver/semver"
)

// Migrate applies any necessary migrations, returning a new migrated
// version of the Function.  It is the caller's responsibility to
// .Write() the Function to persist to disk.
func (f Function) Migrate() (migrated Function, err error) {
	// Return immediately if the Function indicates it has already been
	// migrated.
	if f.Migrated() {
		return f, nil
	}

	// If the version is empty, treat it as 0.0.0
	if f.Version == "" {
		f.Version = DefaultVersion
	}

	migrated = f // initially equivalent
	for _, m := range migrations {
		// Skip this migration if the current function's version is not less than
		// the migration's applicable verion.
		if !semver.New(migrated.Version).LessThan(*semver.New(m.version)) {
			continue
		}
		// Apply this migration when the Function's version is less than that which
		// the migration will impart.
		migrated, err = m.migrate(migrated, m)
		if err != nil {
			return // fail fast on any migration errors
		}
	}
	return
}

// migration is a migration which should be applied to Functions whose version
// is below that indicated.
type migration struct {
	version string   // version before which this migration may be needed.
	migrate migrator // Migrator migrates.
}

// migrator is a function which returns a migrated copy of an inbound function.
type migrator func(Function, migration) (Function, error)

// Migrated returns whether or not the Function has been migrated to the highest
// level the currently executing system is aware of (or beyond).
// returns true.
func (f Function) Migrated() bool {
	// If the function has no Version, it is pre-migrations and is implicitly
	// not migrated.
	if f.Version == "" {
		return false
	}

	// lastMigration is the last registered migration.
	lastMigration := semver.New(migrations[len(migrations)-1].version)

	// Fail the migration test if the Function's version is less than
	// the latest available.
	return !semver.New(f.Version).LessThan(*lastMigration)
}

// Migrations registry
// -------------------

// migrations are all migrators in ascending order.
// No two migrations may have the exact version number (introduce a patch
// version for the migration if necessary)
var migrations = []migration{
	{"0.19.0", migrateToCreationStamp},
	// New Migrations Here.
}

// Individual Migration implementations
// ------------------------------------

// migrateToCreationStamp is the initial migration which brings a Function from
// some unknown point in the past to the point at which it is versioned,
// migrated and includes a creation timestamp.  Without this migration,
// instantiation of old functions will fail with a "Function at path X not
// initialized" in Func versions above v0.19.0
//
// This migration must be aware of the difference between a Function which
// was previously created (but with no create stamp), and a Function which
// exists only in memory and should legitimately fail the .Initialized() check.
// The only way to know is to check a side-effect of earlier versions:
// are the .Name and .Runtime fields populated.  This was the way the
// .Initialized check was implemented prior to versioning being introduced, so
// it is equivalent logically to use this here as well.

// In summary:  if the creation stamp is zero, but name and runtime fields are
// populated, then this is an old Function and should be migrated to having a
// create stamp.  Otherwise, this is an in-memory (new) Function that is
// currently in the process of being created and as such need not be mutated
// to consider this migration having been evaluated.
func migrateToCreationStamp(f Function, m migration) (Function, error) {
	// For functions with no creation timestamp, but appear to have been pre-
	// existing, populate their create stamp and version.
	// Yes, it's a little gnarly, but bootstrapping into the lovelieness of a
	// versioned/migrated system takes cleaning up the trash.
	if f.Created.IsZero() { // If there is no create stamp
		if f.Name != "" && f.Runtime != "" { // and it appears to be an old Function
			f.Created = time.Now() // Migrate it to having a timestamp.
		}
	}
	f.Version = m.version // Record this migration was evaluated.
	return f, nil
}
