package functions

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"gopkg.in/yaml.v2"
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
	expectedGit := Source{URL: "http://test-url", Revision: "test revision", Dir: "/test/context/dir"}
	expectedNamespace := "test-namespace"
	var expectedEnvs []Env
	var expectedVolumes []Volume

	f, err := NewFunction(root)
	if err != nil {
		t.Error(err)
		t.Fatal(f)
	}

	if f.Build.Source != expectedGit {
		t.Fatalf("migrated Function expected Source '%v', got '%v'", expectedGit, f.Build.Source)
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

// TestUnknownFieldsWarning verifies that loading a func.yaml at the latest
// spec version with unknown fields prints a warning to stderr.
// Note we have to do this 'dynamically' because we need the latest spec,
// whatever it is.
func TestUnknownFieldsWarning(t *testing.T) {
	unknownFieldsOnce = sync.Once{}

	root := t.TempDir()
	if err := writeFunc(Function{
		SpecVersion: LastSpecVersion(),
		Name:        "test-func",
		Runtime:     "go",
	}, root); err != nil {
		t.Fatal(err)
	}
	funcYaml, _ := os.OpenFile(root+"/func.yaml", os.O_APPEND|os.O_WRONLY, 0644)
	if _, err := funcYaml.WriteString("junkField: bad\n"); err != nil {
		t.Fatal(err)
	}
	funcYaml.Close()

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	w.Close()
	os.Stderr = old

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if !strings.Contains(output, "Warning") {
		t.Errorf("expected warning in stderr, got: %q", output)
	}
	if !strings.Contains(output, `unknown field "junkField"`) {
		t.Errorf("expected junkField in warning, got: %q", output)
	}
}

// TestUnknownFieldsNoWarningOnClean verifies that a valid func.yaml
// produces no warning.
func TestUnknownFieldsNoWarningOnClean(t *testing.T) {
	unknownFieldsOnce = sync.Once{}

	root := t.TempDir()
	f := Function{
		Runtime: "go",
		Root:    root,
	}
	f.SpecVersion = LastSpecVersion()
	f.Name = "clean-func"
	f.Created = f.Created.Add(1)
	if err := writeFunc(f, root); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	w.Close()
	os.Stderr = old

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if strings.Contains(output, "Warning") {
		t.Errorf("expected no warning for clean func.yaml, got: %q", output)
	}
}

// TestUnknownFieldsNoWarningPreMigration verifies that old func.yaml files
// (not yet at the latest spec) do NOT trigger the unknown fields warning,
// since they may contain old keys that migration handles.
func TestUnknownFieldsNoWarningPreMigration(t *testing.T) {
	unknownFieldsOnce = sync.Once{}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, _ = NewFunction("testdata/migrations/v0.34.0")

	w.Close()
	os.Stderr = old

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if strings.Contains(output, "Warning") {
		t.Errorf("expected no warning for pre-migration func.yaml, got: %q", output)
	}
}

// TestUnknownFieldsWarnOnStaleScale verifies that once Options no longer
// carries a scale field, an at-spec func.yaml that still has a stale
// deploy.options.scale block is surfaced by the unknown-fields warning rather
// than silently accepted. This is the behavior gauron99's G1 comment was after:
// while the key mapped to a struct field it passed strict unmarshal unnoticed.
func TestUnknownFieldsWarnOnStaleScale(t *testing.T) {
	unknownFieldsOnce = sync.Once{}

	root := t.TempDir()
	funcYaml := "specVersion: \"" + LastSpecVersion() + "\"\n" + `name: testfn
runtime: go
created: 2024-01-01T00:00:00Z
deploy:
  options:
    scale:
      min: 1
      max: 3
`
	if err := os.WriteFile(filepath.Join(root, FunctionFile), []byte(funcYaml), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, _ = NewFunction(root)

	w.Close()
	os.Stderr = old

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if !strings.Contains(output, "Warning") {
		t.Errorf("expected an unknown-fields warning for a stale deploy.options.scale, got: %q", output)
	}
	if !strings.Contains(output, "scale") {
		t.Errorf("expected the warning to name the stale scale key, got: %q", output)
	}
}

func writeFunc(f Function, root string) error {
	bb, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	return os.WriteFile(root+"/func.yaml", bb, 0644)
}

// TestMigrateGitToSource ensures the former build.git keys (url, revision,
// contextDir) are carried over into build.source (url, revision, dir), and
// written back under the new keys.
func TestMigrateGitToSource(t *testing.T) {
	f, err := NewFunction("testdata/migrations/v0.37.0")
	if err != nil {
		t.Fatal(err)
	}
	want := Source{URL: "https://example.com/alice/testfunc.git", Revision: "feature", Dir: "functions/testfunc"}
	if f.Build.Source != want {
		t.Fatalf("migrated Function expected source %+v, got %+v", want, f.Build.Source)
	}
	if f.SpecVersion != LastSpecVersion() {
		t.Errorf("expected specVersion %q, got %q", LastSpecVersion(), f.SpecVersion)
	}

	f.Root = t.TempDir()
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	bb, err := os.ReadFile(filepath.Join(f.Root, FunctionFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"source:", "url: https://example.com/alice/testfunc.git", "revision: feature", "dir: functions/testfunc"} {
		if !strings.Contains(string(bb), want) {
			t.Errorf("expected %q in the written func.yaml, got:\n%s", want, bb)
		}
	}
	if strings.Contains(string(bb), "git:") || strings.Contains(string(bb), "contextDir:") {
		t.Errorf("expected no build.git keys in the written func.yaml, got:\n%s", bb)
	}
}

func TestMigrateScaleToTopLevel(t *testing.T) {
	// These tests drive the migration through NewFunction(root) on an inline
	// func.yaml -- the real load path -- rather than calling
	// migrateScaleToTopLevel with a hand-built Function. NewFunction unmarshals
	// the file (populating f.Deployer, f.Deploy.Deployer/Expose, etc.) and runs
	// the full migration chain, so every fixture is a state that can actually
	// occur on disk.
	newFn := func(t *testing.T, funcYaml string) Function {
		t.Helper()
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, FunctionFile), []byte(funcYaml), 0644); err != nil {
			t.Fatal(err)
		}
		f, err := NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}

	t.Run("flat fields move to top-level scale.kpa", func(t *testing.T) {
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deploy:
  options:
    scale:
      min: 1
      max: 10
      metric: concurrency
      target: 100.0
      utilization: 70.0
`)
		if migrated.SpecVersion != LastSpecVersion() {
			t.Errorf("specVersion = %q, want %q", migrated.SpecVersion, LastSpecVersion())
		}
		if migrated.Scale == nil {
			t.Fatal("expected top-level scale to be populated")
		}
		if migrated.Scale.Min == nil || *migrated.Scale.Min != 1 {
			t.Errorf("scale.min = %v, want 1", migrated.Scale.Min)
		}
		if migrated.Scale.Max == nil || *migrated.Scale.Max != 10 {
			t.Errorf("scale.max = %v, want 10", migrated.Scale.Max)
		}
		if migrated.Scale.KPA == nil {
			t.Fatal("expected scale.kpa to be populated from flat fields")
		}
		if *migrated.Scale.KPA.Metric != "concurrency" {
			t.Errorf("scale.kpa.metric = %q, want concurrency", *migrated.Scale.KPA.Metric)
		}
		if *migrated.Scale.KPA.Target != 100.0 {
			t.Errorf("scale.kpa.target = %f, want 100", *migrated.Scale.KPA.Target)
		}
		if *migrated.Scale.KPA.Utilization != 70.0 {
			t.Errorf("scale.kpa.utilization = %f, want 70", *migrated.Scale.KPA.Utilization)
		}
	})

	t.Run("no-op when no scale fields", func(t *testing.T) {
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
`)
		if migrated.SpecVersion != LastSpecVersion() {
			t.Errorf("specVersion = %q, want %q", migrated.SpecVersion, LastSpecVersion())
		}
		if migrated.Scale != nil {
			t.Errorf("expected nil scale, got %+v", migrated.Scale)
		}
	})

	t.Run("non-keda deployer no triggers added", func(t *testing.T) {
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deployer: raw
`)
		if migrated.Scale != nil {
			t.Errorf("expected no scale for raw deployer, got %+v", migrated.Scale)
		}
	})

	t.Run("lifts pre-0.34 top-level options.scale from disk", func(t *testing.T) {
		// Before the specs restructure (0.34.0), scale lived under a top-level
		// options.scale block. migrateToSpecsStructure no longer stages that
		// block in-memory, so migrateScaleToTopLevel reads it directly from
		// disk. Reading it with the old shape also recovers the flat
		// metric/target/utilization fields that the specs migration dropped.
		migrated := newFn(t, `specVersion: "0.33.0"
name: testfn
runtime: go
options:
  scale:
    min: 2
    max: 20
    metric: concurrency
    target: 100
    utilization: 70
`)
		if migrated.Scale == nil {
			t.Fatal("expected the pre-0.34 top-level scale to be lifted, got nil")
		}
		if migrated.Scale.Min == nil || *migrated.Scale.Min != 2 {
			t.Errorf("scale.min = %v, want 2", migrated.Scale.Min)
		}
		if migrated.Scale.Max == nil || *migrated.Scale.Max != 20 {
			t.Errorf("scale.max = %v, want 20", migrated.Scale.Max)
		}
		if migrated.Scale.KPA == nil {
			t.Fatal("expected scale.kpa to be recovered from the pre-0.34 flat fields")
		}
		if migrated.Scale.KPA.Metric == nil || *migrated.Scale.KPA.Metric != "concurrency" {
			t.Errorf("scale.kpa.metric = %v, want concurrency", migrated.Scale.KPA.Metric)
		}
		if migrated.Scale.KPA.Target == nil || *migrated.Scale.KPA.Target != 100.0 {
			t.Errorf("scale.kpa.target = %v, want 100", migrated.Scale.KPA.Target)
		}
		if migrated.Scale.KPA.Utilization == nil || *migrated.Scale.KPA.Utilization != 70.0 {
			t.Errorf("scale.kpa.utilization = %v, want 70", migrated.Scale.KPA.Utilization)
		}
	})

	t.Run("legacy flat fields move to scale.kpa for a non-knative deployer too", func(t *testing.T) {
		// The migration is a plain move: it lifts the legacy flat
		// metric/target/utilization fields into scale.kpa without inspecting the
		// deployer. scale.kpa on a raw function is not a validation error -- it
		// is ignored with a warning at deploy time (see warnScaleKpaIgnore).
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deployer: raw
deploy:
  options:
    scale:
      metric: concurrency
      target: 100.0
      utilization: 70.0
`)
		if migrated.Scale == nil || migrated.Scale.KPA == nil {
			t.Fatalf("expected scale.kpa to be lifted for deployer: raw, got %+v", migrated.Scale)
		}
		if migrated.Scale.KPA.Metric == nil || *migrated.Scale.KPA.Metric != "concurrency" {
			t.Errorf("scale.kpa.metric = %v, want concurrency", migrated.Scale.KPA.Metric)
		}
		if errs := ValidateScale(migrated.Scale, "raw"); len(errs) != 0 {
			t.Errorf("expected the migrated scale to pass validation, got: %v", errs)
		}
	})

	t.Run("legacy flat fields move to scale.kpa when the deployer is recorded only under deploy.deployer", func(t *testing.T) {
		// Pre-#3953 files recorded the deployer intent only under
		// deploy.deployer. The migration is deployer-agnostic, so the legacy
		// flat fields are lifted into scale.kpa regardless; keda ignores it with
		// a warning at deploy time.
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deploy:
  deployer: keda
  options:
    scale:
      metric: concurrency
      target: 100.0
      utilization: 70.0
`)
		if migrated.Deploy.Deployer != "keda" {
			t.Errorf("Deploy.Deployer = %q, want keda", migrated.Deploy.Deployer)
		}
		if migrated.Scale == nil || migrated.Scale.KPA == nil {
			t.Fatalf("expected scale.kpa to be lifted for a keda-observed function, got %+v", migrated.Scale)
		}
		if migrated.Scale.KPA.Metric == nil || *migrated.Scale.KPA.Metric != "concurrency" {
			t.Errorf("scale.kpa.metric = %v, want concurrency", migrated.Scale.KPA.Metric)
		}
		if errs := ValidateScale(migrated.Scale, "keda"); len(errs) != 0 {
			t.Errorf("expected the migrated scale to pass keda validation, got: %v", errs)
		}
	})

	t.Run("deploy.deployer/deploy.expose survive migration without becoming intent", func(t *testing.T) {
		// A pre-#3953 legacy file recorded the deployer only under the old
		// deploy.deployer key. Those observed-state fields must survive the
		// migration, but it must NOT promote the legacy value to f.Deployer
		// (intent) -- that recovery was removed as an unrelated, pre-existing
		// concern (see the follow-up ticket).
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deploy:
  deployer: keda
  expose: route
`)
		if migrated.Deploy.Deployer != "keda" {
			t.Errorf("Deploy.Deployer = %q, want keda", migrated.Deploy.Deployer)
		}
		if migrated.Deploy.Expose != "route" {
			t.Errorf("Deploy.Expose = %q, want route", migrated.Deploy.Expose)
		}
		// Intent is left empty: the migration no longer recovers the legacy
		// deploy.deployer value as f.Deployer.
		if migrated.Deployer != "" {
			t.Errorf("Deployer = %q, want empty (legacy intent recovery removed)", migrated.Deployer)
		}
	})

	t.Run("migration never touches f.Deployer intent", func(t *testing.T) {
		// The migration must leave the intent field exactly as it was loaded --
		// it neither invents intent from the legacy observed field nor overrides
		// an already-present one.
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
deployer: raw
deploy:
  deployer: knative
`)
		if migrated.Deployer != "raw" {
			t.Errorf("Deployer = %q, want raw (must be left untouched)", migrated.Deployer)
		}
		if migrated.Deploy.Deployer != "knative" {
			t.Errorf("Deploy.Deployer = %q, want knative", migrated.Deploy.Deployer)
		}
	})

	t.Run("no-op when neither old deployer/expose key is present", func(t *testing.T) {
		migrated := newFn(t, `specVersion: "0.36.0"
name: testfn
runtime: go
`)
		if migrated.Deploy.Deployer != "" || migrated.Deploy.Expose != "" {
			t.Errorf("expected both fields to stay empty, got Deployer=%q Expose=%q",
				migrated.Deploy.Deployer, migrated.Deploy.Expose)
		}
	})
}
