package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"

	. "knative.dev/func/pkg/testing"
)

// TestNewDefaults ensures that the default Config
// constructor yelds a struct prepopulated with static
// defaults.
func TestNewDefaults(t *testing.T) {
	cfg := config.New()
	if cfg.Language != config.DefaultLanguage {
		t.Fatalf("expected config's language = '%v', got '%v'", config.DefaultLanguage, cfg.Language)
	}
}

// TestLoad ensures that loading a config reads values
// in from a config file at path, and in this case (unlike NewDefault) the
// file must exist at path or error.
func TestLoad(t *testing.T) {
	cfg, err := config.Load(filepath.Join("testdata", "TestLoad", "func", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "custom" {
		t.Fatalf("loaded config did not contain values from config file.  Expected \"custom\" got \"%v\"", cfg.Language)
	}

	// and ensure error
	cfg, err = config.Load("invalid/path")
	if err == nil {
		t.Fatal("did not receive expected error loading nonexistent config path")
	}
}

// TestWrite ensures that writing a config persists.
func TestWrite(t *testing.T) {
	root, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", root)
	var err error

	// Ensure error writing when config paths do not exist
	cfg := config.New()
	cfg.Language = "example"
	if err = cfg.Write(config.File()); err == nil {
		t.Fatal("did not receive error writing to a nonexistent path")
	}

	// Create the path and ensure writing generates no error
	if err = config.CreatePaths(); err != nil {
		t.Fatal(err)
	}
	if err = cfg.Write(config.File()); err != nil {
		t.Fatal(err)
	}

	// Confirm value was persisted
	if cfg, err = config.Load(config.File()); err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "example" {
		t.Fatalf("config did not persist.  expected 'example', got '%v'", cfg.Language)
	}
}

// TestPath ensures that the Path accessor returns
// XDG_CONFIG_HOME/.config/func
func TestPath(t *testing.T) {
	home := t.TempDir()                 // root of all configs
	path := filepath.Join(home, "func") // our config

	t.Setenv("XDG_CONFIG_HOME", home)

	if config.Dir() != path {
		t.Fatalf("expected config path '%v', got '%v'", path, config.Dir())
	}
}

// TestNewDefault ensures that the default returned from NewDefault includes
// both the static defaults (see TestNewDefaults), as well as those from the
// currently effective global config path (~/config/func).
func TestNewDefault(t *testing.T) {
	// Custom config home results in a config file default path of
	// ./testdata/func/config.yaml
	home := filepath.Join(Cwd(), "testdata")
	t.Setenv("XDG_CONFIG_HOME", home)

	cfg, err := config.NewDefault() // Should load values from above config
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Language != "custom" {
		t.Fatalf("config file not loaded")
	}
}

// TestCreatePaths ensures that the paths are created when requested.
func TestCreatePaths(t *testing.T) {
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)

	t.Setenv("XDG_CONFIG_HOME", home)

	if err := config.CreatePaths(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(config.Dir()); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("config path '%v' not created", config.Dir())
		}
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(config.Dir(), "repositories")); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("config path '%v' not created", config.Dir())
		}
		t.Fatal(err)
	}

	// Trying to create when repositories path is invalid should error
	_ = os.WriteFile("./invalidRepositoriesPath.txt", []byte{}, os.ModePerm)
	t.Setenv("FUNC_REPOSITORIES_PATH", "./invalidRepositoriesPath.txt")
	if err := config.CreatePaths(); err == nil {
		t.Fatal("did not receive error when creating paths with an invalid FUNC_REPOSITORIES_PATH")
	}

	// Trying to Create config path should bubble errors, for example when HOME is
	// set to a nonexistent path.
	_ = os.WriteFile("./invalidConfigHome.txt", []byte{}, os.ModePerm)
	t.Setenv("XDG_CONFIG_HOME", "./invalidConfigHome.txt")
	if err := config.CreatePaths(); err == nil {
		t.Fatal("did not receive error when creating paths in an invalid home")
	}
}

// TestNewDefault_ConfigNotRequired ensures that when creating a new
// config which would load a global config, its nonexistence causes no error.
func TestNewDefault_ConfigNotRequired(t *testing.T) {
	// Custom config home results in a config file default path of
	// ./testdata/func/config.yaml
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", home)

	_, err := config.NewDefault() // Should not error despite no config.
	if err != nil {
		t.Fatal(err)
	}
}

// TestRepositoriesPath returns the path expected
// (XDG_CONFIG_HOME/func/repositories by default)
func TestRepositoriesPath(t *testing.T) {
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("XDG_CONFIG_HOME", home)

	expected := filepath.Join(home, "func", config.Repositories)
	if config.RepositoriesPath() != expected {
		t.Fatalf("unexpected reposiories path: %v", config.RepositoriesPath())
	}
}

// TestDefaultNamespace ensures that, when there is a problem determining the
// active namespace, the static DefaultNamespace ("default") is used and that
// the currently active k8s namespace is used as the default if available.
func TestDefaultNamespace(t *testing.T) {
	cwd := Cwd() // store for use after Mktemp which changes working directory

	// Namespace "default" when empty home
	// Note that KUBECONFIG must be defined, or the current user's ~/.kube/config
	// will be used (and thus whichever namespace they have currently active)
	home, cleanup := Mktemp(t)
	t.Cleanup(cleanup)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("XDG_CONFIG_HOME", home)
	if config.DefaultNamespace() != "default" {
		t.Fatalf("did not receive expected default namespace 'default', got '%v'", config.DefaultNamespace())
	}

	// should be "func" when active k8s namespace is "func"
	kubeconfig := filepath.Join(cwd, "testdata", "TestDefaultNamespace", "kubeconfig")
	t.Setenv("KUBECONFIG", kubeconfig)
	if config.DefaultNamespace() != "func" {
		t.Fatalf("expected default namespace of 'func' when that is the active k8s namespace.  Got '%v'", config.DefaultNamespace())
	}
}

// TestApply ensures that applying a function as context to a config results
// in every member of config in the intersection of the two sets, global config
// and function, to be set to the values of the function.
// (See the associated cfg.Configure)
func TestApply(t *testing.T) {
	// Yes, every member needs to be painstakingly enumerated by hand, because
	// the sets are not equivalent.  Not all global settings have an associated
	// member on the function (example: confirm), and not all members of a
	// function are globally configurable (example: image).
	f := fn.Function{
		Build: fn.BuildSpec{
			Builder: "builder",
		},
		Deploy: fn.DeploySpec{
			Namespace: "namespace",
		},
		Runtime:  "runtime",
		Registry: "registry",
	}
	cfg := config.Global{}.Apply(f)

	if cfg.Builder != "builder" {
		t.Error("apply missing map of f.Build.Builder")
	}
	if cfg.Language != "runtime" {
		t.Error("apply missing map of f.Runtime ")
	}
	if cfg.Namespace != "namespace" {
		t.Error("apply missing map of f.Namespace")
	}
	if cfg.Registry != "registry" {
		t.Error("apply missing map of f.Registry")
	}

	// empty values in the function context should not zero out
	// populated values in the global config when applying.
	cfg.Apply(fn.Function{})
	if cfg.Builder == "" {
		t.Error("empty f.Build.Builder should not be mapped")
	}
	if cfg.Language == "" {
		t.Error("empty f.Runtime should not be mapped")
	}
	if cfg.Namespace == "" {
		t.Error("empty f.Namespace should not be mapped")
	}
	if cfg.Registry == "" {
		t.Error("empty f.Registry should not be mapped")
	}
}

// TestConfigyre ensures that configuring a function results in every member
// of the function in the intersection of the two sets, global config and function
// members, to be set to the values of the config.
// (See the associated cfg.Apply)
func TestConfigure(t *testing.T) {
	f := fn.Function{}
	cfg := config.Global{
		Builder:   "builder",
		Language:  "runtime",
		Namespace: "namespace",
		Registry:  "registry",
	}
	f = cfg.Configure(f)

	if f.Build.Builder != "builder" {
		t.Error("configure missing map for f.Build.Builder")
	}
	if f.Deploy.Namespace != "namespace" {
		t.Error("configure missing map for f.Deploy.Namespace")
	}
	if f.Runtime != "runtime" {
		t.Error("configure missing map for f.Language")
	}
	if f.Registry != "registry" {
		t.Error("configure missing map for f.Registry")
	}

	// empty values in the global config shoul not zero out function values
	// when configuring.
	f = config.Global{}.Configure(f)
	if f.Build.Builder == "" {
		t.Error("empty cfg.Builder should not mutate f")
	}
	if f.Deploy.Namespace == "" {
		t.Error("empty cfg.Namespace should not mutate f")
	}
	if f.Runtime == "" {
		t.Error("empty cfg.Runtime should not mutate f")
	}
	if f.Registry == "" {
		t.Error("empty cfg.Registry should not mutate f")
	}

}

// TestGet_Invalid ensures that attempting to get the value of a nonexistent
// member returns nil.
func TestGet_Invalid(t *testing.T) {
	v := config.Get(config.Global{}, "invalid")
	if v != nil {
		t.Fatalf("expected accessing a nonexistent member to return nil, but got: %v", v)
	}
}

// TestGet_Valid ensures a valid field name returns the value for that field.
// Name is keyed off the yaml serialization key of the field rather than the
// (capitalized) exported member name of the struct in order to be consistent
// with the disk-serialized config file format, and thus integrate nicely with
// CLIs, etc.
func TestGet_Valid(t *testing.T) {
	c := config.Global{
		Builder: "myBuilder",
		Confirm: true,
	}
	// Get String
	v := config.Get(c, "builder")
	if v != "myBuilder" {
		t.Fatalf("Did not receive expected value for builder.  got: %v", v)
	}
	// Get Boolean
	v = config.Get(c, "confirm")
	if v != true {
		t.Fatalf("Did not receive expected value for builder.  got: %v", v)
	}
}

// TestSet_Invalid ensures that attemptint to set an invalid field errors.
func TestSet_Invalid(t *testing.T) {
	_, err := config.SetString(config.Global{}, "invalid", "foo")
	if err == nil {
		t.Fatal("did not receive expected error setting a nonexistent field")
	}
}

// TestSet_ValidTyped ensures that attempting to set attributes with valid
// names and typed values succeeds.
func TestSet_ValidTyped(t *testing.T) {
	cfg := config.Global{}

	// Set a String
	cfg, err := config.SetString(cfg, "builder", "myBuilder")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Builder != "myBuilder" {
		t.Fatalf("unexpected value for config builder: %v", cfg.Builder)
	}

	// Set a Bool
	cfg, err = config.SetBool(cfg, "confirm", true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Builder != "myBuilder" {
		t.Fatalf("unexpected value for config builder: %v", cfg.Builder)
	}

	// TODO lazily populate typed accessors if/when global config expands to
	// include types of additional values.
}

// TestSet_ValidStrings ensures that setting valid attribute names using
// the string representation of their values succeeds.
func TestSet_ValidStrings(t *testing.T) {
	cfg := config.Global{}

	// Set a String from a string
	// should be the base case
	cfg, err := config.Set(cfg, "builder", "myBuilder")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Builder != "myBuilder" {
		t.Fatalf("unexpected value for config builder: %v", cfg.Builder)
	}

	// Set a Bool
	cfg, err = config.SetBool(cfg, "confirm", true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Builder != "myBuilder" {
		t.Fatalf("unexpected value for config builder: %v", cfg.Builder)
	}

	// TODO: lazily populate support of additional types in the implementation
	// as needed.
}

// TestList ensures that the expected result is returned when listing
// the current names and values of the global config.
// The name is the name that can be used with Get and Set.  The value is the
// string serialization of the value for the given name.
func TestList(t *testing.T) {
	values := config.List()
	expected := []string{
		"builder",
		"confirm",
		"language",
		"namespace",
		"registry",
		"registryInsecure",
		"verbose",
	}

	if !reflect.DeepEqual(values, expected) {
		t.Logf("expected:\n%v", expected)
		t.Logf("received:\n%v", values)
		t.Fatalf("unexpected list of configurable options.")
	}

	// NOTE: due to the strictness of this test, a new slice member will need
	// to be added for each new field added to global config.

}
