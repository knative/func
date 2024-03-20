package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ory/viper"
	"knative.dev/client-pkg/pkg/util"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

const TestRegistry = "example.com/alice"

func TestRoot_mergeEnvMaps(t *testing.T) {

	a := "A"
	b := "B"
	v1 := "x"
	v2 := "y"

	type args struct {
		envs     []fn.Env
		toUpdate *util.OrderedMap
		toRemove []string
	}
	tests := []struct {
		name string
		args args
		want []fn.Env
	}{
		{
			"add new var to empty list",
			args{
				[]fn.Env{},
				util.NewOrderedMapWithKVStrings([][]string{{a, v1}}),
				[]string{},
			},
			[]fn.Env{{Name: &a, Value: &v1}},
		},
		{
			"add new var",
			args{
				[]fn.Env{{Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v1}}),
				[]string{},
			},
			[]fn.Env{{Name: &b, Value: &v2}, {Name: &a, Value: &v1}},
		},
		{
			"update var",
			args{
				[]fn.Env{{Name: &a, Value: &v1}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}}),
				[]string{},
			},
			[]fn.Env{{Name: &a, Value: &v2}},
		},
		{
			"update multiple vars",
			args{
				[]fn.Env{{Name: &a, Value: &v1}, {Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}, {b, v1}}),
				[]string{},
			},
			[]fn.Env{{Name: &a, Value: &v2}, {Name: &b, Value: &v1}},
		},
		{
			"remove var",
			args{
				[]fn.Env{{Name: &a, Value: &v1}},
				util.NewOrderedMap(),
				[]string{a},
			},
			[]fn.Env{},
		},
		{
			"remove multiple vars",
			args{
				[]fn.Env{{Name: &a, Value: &v1}, {Name: &b, Value: &v2}},
				util.NewOrderedMap(),
				[]string{a, b},
			},
			[]fn.Env{},
		},
		{
			"update and remove vars",
			args{
				[]fn.Env{{Name: &a, Value: &v1}, {Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}}),
				[]string{b},
			},
			[]fn.Env{{Name: &a, Value: &v2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := mergeEnvs(tt.args.envs, tt.args.toUpdate, tt.args.toRemove)
			if err != nil {
				t.Errorf("mergeEnvs() for initial vars %v and toUpdate %v and toRemove %v got error %v",
					tt.args.envs, tt.args.toUpdate, tt.args.toRemove, err)
			}
			if !reflect.DeepEqual(got, tt.want) {

				gotString := "{ "
				for _, e := range got {
					gotString += fmt.Sprintf("{ %s: %s } ", *e.Name, *e.Value)
				}
				gotString += "}"

				wantString := "{ "
				for _, e := range tt.want {
					wantString += fmt.Sprintf("{ %s: %s } ", *e.Name, *e.Value)
				}
				wantString += "}"

				t.Errorf("mergeEnvs() = got: %s, want %s", gotString, wantString)
			}
		})
	}
}

// TestRoot_CommandNameParameterized confirmst that the command name, as
// printed in help text, is parameterized based on the constructor parameters
// of the root command.  This allows, for example, to have help text correct
// when both embedded as a plugin or standalone.
func TestRoot_CommandNameParameterized(t *testing.T) {
	expectedSynopsis := "%v is the command line interface for"

	tests := []string{
		"func",    // standalone
		"kn func", // kn plugin
	}

	for _, testName := range tests {
		var (
			cmd = NewRootCmd(RootCommandConfig{Name: testName})
			out = strings.Builder{}
		)
		cmd.SetArgs([]string{}) // Do not use test command args
		cmd.SetOut(&out)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		if cmd.Use != testName {
			t.Fatalf("expected command Use '%v', got '%v'", testName, cmd.Use)
		}
		if !strings.HasPrefix(out.String(), fmt.Sprintf(expectedSynopsis, testName)) {
			t.Logf("Testing '%v'\n", testName)
			t.Log(out.String())
			t.Fatalf("Help text does not include substituted name '%v'", testName)
		}
	}
}

func TestVerbose(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		want   string
		wantLF int
	}{
		{
			name:   "verbose as version's flag",
			args:   []string{"version", "-v"},
			want:   "Version: v0.42.0",
			wantLF: 6,
		},
		{
			name:   "no verbose",
			args:   []string{"version"},
			want:   "v0.42.0",
			wantLF: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			var out bytes.Buffer

			cmd := NewRootCmd(RootCommandConfig{
				Name: "func",
				Version: Version{
					Vers: "v0.42.0",
					Hash: "cafe",
					Kver: "v1.10.0",
				}})

			cmd.SetArgs(tt.args)
			cmd.SetOut(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			outLines := strings.Split(out.String(), "\n")
			if len(outLines)-1 != tt.wantLF {
				t.Errorf("expected output with %v line breaks but got %v:", tt.wantLF, len(outLines)-1)
			}
			if outLines[0] != tt.want {
				t.Errorf("expected output: %q but got: %q", tt.want, outLines[0])
			}
		})
	}
}

// TestRoot_effectivePath ensures that the path method returns the effective path
// to use with the following precedence:  empty by default, then FUNC_PATH
// environment variable, -p flag, or finally --path with the highest precedence.
func TestRoot_effectivePath(t *testing.T) {

	args := os.Args
	t.Cleanup(func() { os.Args = args })

	t.Run("default", func(t *testing.T) {
		if effectivePath() != "" {
			t.Fatalf("the default path should be '.', got '%v'", effectivePath())
		}
	})

	t.Run("FUNC_PATH", func(t *testing.T) {
		t.Setenv("FUNC_PATH", "p1")
		if effectivePath() != "p1" {
			t.Fatalf("the effetive path did not load the environment variable.  Expected 'p1', got '%v'", effectivePath())
		}
	})

	t.Run("--path", func(t *testing.T) {
		os.Args = []string{"test", "--path=p2"}
		if effectivePath() != "p2" {
			t.Fatalf("the effective path did not load the --path flag.  Expected 'p2', got '%v'", effectivePath())
		}
	})

	t.Run("-p", func(t *testing.T) {
		os.Args = []string{"test", "-p=p3"}
		if effectivePath() != "p3" {
			t.Fatalf("the effective path did not load the -p flag.  Expected 'p3', got '%v'", effectivePath())
		}
	})

	t.Run("short flag precedence", func(t *testing.T) {
		t.Setenv("FUNC_PATH", "p1")
		os.Args = []string{"test", "-p=p3"}
		if effectivePath() != "p3" {
			t.Fatalf("the effective path did not load the -p flag with precedence over FUNC_PATH.  Expected 'p3', got '%v'", effectivePath())
		}
	})

	t.Run("-p highest precedence", func(t *testing.T) {
		t.Setenv("FUNC_PATH", "p1")
		os.Args = []string{"test", "--path=p2", "-p=p3"}
		if effectivePath() != "p3" {
			t.Fatalf("the effective path did not take -p with highest precedence over --path and FUNC_PATH.  Expected 'p3', got '%v'", effectivePath())
		}
	})

	t.Run("continues on unrecognized flags", func(t *testing.T) {
		os.Args = []string{"test", "-r=repo.example.com/bob", "-p=p3"}
		if effectivePath() != "p3" {
			t.Fatalf("the effective path did not evaluate when unexpected flags were present")
		}
	})

}

// Helpers
// -------

// pipe the output of stdout to a buffer whose value is returned
// from the returned function.  Call pipe() to start piping output
// to the buffer, call the returned function to access the data in
// the buffer.
func piped(t *testing.T) func() string {
	t.Helper()
	var (
		o = os.Stdout
		c = make(chan error, 1)
		b strings.Builder
	)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stdout = w

	go func() {
		_, err := io.Copy(&b, r)
		r.Close()
		c <- err
	}()

	return func() string {
		os.Stdout = o
		w.Close()
		err := <-c
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(b.String())
	}
}

// fromTempDirectory is a test helper which endeavors to create
// an environment clean of developer's settings for use during CLI testing.
func fromTempDirectory(t *testing.T) string {
	t.Helper()
	ClearEnvs(t)

	// We have to define KUBECONFIG, or the file at ~/.kube/config (if extant)
	// will be used (disrupting tests by using the current user's environment).
	// The test kubeconfig set below has the current namespace set to 'func'
	// NOTE: the below settings affect unit tests only, and we do explicitly
	// want all unit tests to start in an empty environment with tests "opting in"
	// to config, not opting out.
	t.Setenv("KUBECONFIG", filepath.Join(cwd(), "testdata", "default_kubeconfig"))

	// By default unit tests presum no config exists unless provided in testdata.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	// creates and CDs to a temp directory
	d, done := Mktemp(t)

	// Return to original directory and resets viper.
	t.Cleanup(func() { done(); viper.Reset() })
	return d
}
