package function

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/go-semver/semver"
	"gopkg.in/yaml.v2"
)

// Migrate applies any necessary migrations, returning a new migrated
// version of the function.  It is the caller's responsibility to
// .Write() the function to persist to disk.
func (f Function) Migrate() (migrated Function, err error) {
	// Return immediately if the function indicates it has already been
	// migrated.
	if f.Migrated() {
		return f, nil
	}

	migrated = f // initially equivalent
	for _, m := range migrations {
		// Skip this migration if the current function's specVersion is not less than
		// the migration's applicable specVersion.
		if f.SpecVersion != "" && !semver.New(migrated.SpecVersion).LessThan(*semver.New(m.version)) {
			continue
		}
		// Apply this migration when the function's specVersion is less than that which
		// the migration will impart.
		migrated, err = m.migrate(migrated, m)
		if err != nil {
			return // fail fast on any migration errors
		}
	}
	return
}

// migration is a migration which should be applied to function's whose version
// is below that indicated.
type migration struct {
	version string   // version before which this migration may be needed.
	migrate migrator // Migrator migrates.
}

// migrator is a function which returns a migrated copy of an inbound function.
type migrator func(Function, migration) (Function, error)

// Migrated returns whether the function has been migrated to the highest
// level the currently executing system is aware of (or beyond).
// returns true.
func (f Function) Migrated() bool {
	// If the function has no specVersion, it is pre-migrations and is implicitly
	// not migrated.
	if f.SpecVersion == "" {
		return false
	}

	// lastMigration is the last registered migration.
	lastMigration := semver.New(LastSpecVersion())

	// Fail the migration test if the function's version is less than
	// the latest available.
	return !semver.New(f.SpecVersion).LessThan(*lastMigration)
}

// LastSpecVersion returns the string value for the most recent migration
func LastSpecVersion() string {
	return migrations[len(migrations)-1].version
}

// Migrations registry
// -------------------

// migrations are all migrators in ascending order.
// No two migrations may have the exact version number (introduce a patch
// version for the migration if necessary)
var migrations = []migration{
	{"0.19.0", migrateToCreationStamp},
	{"0.23.0", migrateToBuilderImages},
	{"0.25.0", migrateToSpecVersion},
	{"0.34.0", migrateToSpecsStructure},
	// New Migrations Here.
}

// Individual Migration implementations
// ------------------------------------

// migrateToCreationStamp
// The initial migration which brings a function from
// some unknown point in the past to the point at which it is versioned,
// migrated and includes a creation timestamp.  Without this migration,
// instantiation of old functions will fail with a "Function at path X not
// initialized" in func versions above v0.19.0
//
// This migration must be aware of the difference between a function which
// was previously created (but with no created stamp), and a function which
// exists only in memory and should legitimately fail the .Initialized() check.
// The only way to know is to check a side effect of earlier versions:
// are the `.Name` and `.Runtime` fields populated.  This was the way the
// `.Initialized` check was implemented prior to versioning being introduced, so
// it is equivalent logically to use this here as well.

// In summary:  if the creation stamp is zero, but name and runtime fields are
// populated, then this is an old function and should be migrated to having a
// created stamp.  Otherwise, this is an in-memory (new) function that is
// currently in the process of being created and as such need not be mutated
// to consider this migration having been evaluated.
func migrateToCreationStamp(f Function, m migration) (Function, error) {
	// For functions with no creation timestamp, but appear to have been pre-
	// existing, populate their created stamp and version.
	// Yes, it's a little gnarly, but bootstrapping into the loveliness of a
	// versioned/migrated system takes cleaning up the trash.
	if f.Created.IsZero() { // If there is no created stamp
		if f.Name != "" && f.Runtime != "" { // and it appears to be an old function
			f.Created = time.Now() // Migrate it to having a timestamp.
		}
	}
	f.SpecVersion = m.version // Record this migration was evaluated.
	return f, nil
}

// migrateToBuilderImages
// Prior to this migration, 'builder' and 'builders' attributes of a function
// were specific to buildpack builds.  In addition, the separation of the two
// fields was to facilitate the use of "named" inbuilt builders, which ended
// up not being necessary.  With the addition of the S2I builder implementation,
// it is necessary to differentiate builders for use when building via Pack vs
// builder for use when building with S2I.  Furthermore, now that the builder
// itself is a user-supplied parameter, the short-hand of calling builder images
// simply "builder" is not possible, since that term more correctly refers to
// the builder being used (S2I, pack, or some future implementation), while this
// field very specifically refers to the image the chosen builder should use
// (in leau of the inbuilt default).
//
// For an example of the situation:  the 'builder' member used to instruct the
// system to use that builder _image_ in all cases.  This of course restricts
// the system from being able to build with anything other than the builder
// implementation to which that builder image applies (pack or s2i).  Further,
// always including this value in the serialized func.yaml causes this value to
// be pegged/immutable (without manual intervention), which hampers our ability
// to change out the underlying builder image with future versions.
//
// The 'builder' and 'builders' members have therefore been removed in favor
// of 'builderImages', which is keyed by the short name of the builder
// implementation (currently 'pack' and 's2i').  Its existence is optional,
// with the default value being provided in the associated builder's impl.
// Should the value exist, this indicates the user has overridden the value,
// or is using a fully custom language pack.
//
// This migration allows pre-builder-image functions to load despite their
// inclusion of the now removed 'builder' member.  If the user had provided
// a customized builder image, that value is preserved as the builder image
// for the 'pack' builder in the new version (s2i did not exist prior).
// See associated unit tests.
func migrateToBuilderImages(f1 Function, m migration) (Function, error) {
	// Load the function using pertinent parts of the previous version's schema:
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateToBuilderImages' error: " + err.Error())
	}
	f0 := migrateToBuilderImages_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateToBuilderImages' error: " + err.Error())
	}

	// At time of this migration, the default pack builder image for all language
	// runtimes is:
	defaultPackBuilderImage := "gcr.io/paketo-buildpacks/builder:base"

	// If the old function had defined something custom
	if f0.Builder != "" && f0.Builder != defaultPackBuilderImage {
		// carry it forward as the new pack builder image
		if f1.Build.BuilderImages == nil {
			f1.Build.BuilderImages = make(map[string]string)
		}
		f1.Build.BuilderImages["pack"] = f0.Builder
	}

	// Flag f1 as having had the migration applied
	f1.SpecVersion = m.version
	return f1, nil

}

// migrateToSpecVersion updates a func.yaml file to use SpecVersion
// instead of Version to track the migration numbers
func migrateToSpecVersion(f Function, m migration) (Function, error) {
	// Load the function func.yaml file
	f0Filename := filepath.Join(f.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f, errors.New("migration 'migrateToSpecVersion' error: " + err.Error())
	}

	// Only handle the Version field if it exists
	f0 := migrateToSpecVersion_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f, errors.New("migration 'migrateToSpecVersion' error: " + err.Error())
	}

	f.SpecVersion = m.version
	return f, nil
}

// migrateToSpecsStructure migration makes sure use the sub-specs structs for build, run and deploy phases.
// To avoid unmarshalling issues with the old format this migration needs to be executed first.
// Further migrations will operate on this new struct with sub-specs
func migrateToSpecsStructure(f1 Function, m migration) (Function, error) {
	// Load the Function using pertinent parts of the previous version's schema:
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateToSpecsStructure' error: " + err.Error())
	}
	f0 := migrateToSpecs_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateToSpecsStructure' error: " + err.Error())
	}

	if f0.Git.URL != "" {
		f1.Build.Git.URL = f0.Git.URL
	}
	if f0.Git.Revision != "" {
		f1.Build.Git.Revision = f0.Git.Revision
	}
	if f0.Git.ContextDir != "" {
		f1.Build.Git.ContextDir = f0.Git.ContextDir
	}
	//Append BuilderImages from old format, without destroying previous migrations
	if f0.BuilderImages != nil {
		for k, v := range f0.BuilderImages {
			f1.Build.BuilderImages[k] = v
		}
	}
	if f0.Buildpacks != nil {
		f1.Build.Buildpacks = append(f1.Build.Buildpacks, f0.Buildpacks...)
	}
	if f0.BuildEnvs != nil {
		f1.Build.BuildEnvs = append(f1.Build.BuildEnvs, f0.BuildEnvs...)
	}

	if f0.Volumes != nil {
		f1.Run.Volumes = append(f1.Run.Volumes, f0.Volumes...)
	}

	if f0.Envs != nil {
		f1.Run.Envs = append(f1.Run.Envs, f0.Envs...)
	}

	if f0.Annotations != nil {
		for k, v := range f0.Annotations {
			f1.Deploy.Annotations[k] = v
		}
	}

	if f0.Options.Resources != nil {
		f1.Deploy.Options.Resources = f0.Options.Resources
	}

	if f0.Options.Scale != nil {
		f1.Deploy.Options.Scale = f0.Options.Scale
	}

	if f0.Labels != nil {
		f1.Deploy.Labels = append(f1.Deploy.Labels, f0.Labels...)
	}

	if f0.HealthEndpoints.Readiness != "" {
		f1.Deploy.HealthEndpoints.Readiness = f0.HealthEndpoints.Readiness
	}

	if f0.HealthEndpoints.Liveness != "" {
		f1.Deploy.HealthEndpoints.Liveness = f0.HealthEndpoints.Liveness
	}

	f1.Deploy.Namespace = f0.Namespace
	f1.Build.Builder = f0.Builder
	f1.SpecVersion = m.version
	return f1, nil
}

// The pertinent aspects of the Function's schema prior the 1.0.0 version migrations
type migrateToSpecs_previousFunction struct {

	// Namespace into which the function is deployed on supported platforms.
	Namespace string `yaml:"namespace"`

	// Git stores information about remote git repository,
	// in case build type "git" is being used
	Git Git `yaml:"git"`

	// BuilderImages define optional explicit builder images to use by
	// builder implementations in leau of the in-code defaults.  They key
	// is the builder's short name.  For example:
	// builderImages:
	//   pack: example.com/user/my-pack-node-builder
	//   s2i: example.com/user/my-s2i-node-builder
	BuilderImages map[string]string `yaml:"builderImages,omitempty"`

	// Optional list of buildpacks to use when building the function
	Buildpacks []string `yaml:"buildpacks"`

	// Builder is the name of the subsystem that will complete the underlying
	// build (pack, s2i, etc)
	Builder string `yaml:"builder" jsonschema:"enum=pack,enum=s2i"`

	// List of volumes to be mounted to the function
	Volumes []Volume `yaml:"volumes"`

	// Build Env variables to be set
	BuildEnvs []Env `yaml:"buildEnvs"`

	// Env variables to be set
	Envs []Env `yaml:"envs"`

	// Map containing user-supplied annotations
	// Example: { "division": "finance" }
	Annotations map[string]string `yaml:"annotations"`

	// Options to be set on deployed function (scaling, etc.)
	Options Options `yaml:"options"`

	// Map of user-supplied labels
	Labels []Label `yaml:"labels"`

	// Health endpoints specified by the language pack
	HealthEndpoints HealthEndpoints `yaml:"healthEndpoints"`
}

// Functions prior to 0.25 will have a Version field
type migrateToSpecVersion_previousFunction struct {
	Version string `yaml:"version"`
}

// The pertinent aspects of the function schema prior to the builder images
// migration.
type migrateToBuilderImages_previousFunction struct {
	Builder string `yaml:"builder"`
}
