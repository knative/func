package cmd

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ory/viper"
	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
)

// TestRoot_PersistentFlags ensures that the two global (persistent) flags
// Verbose and Namespace are propagated.
func TestRoot_PersistentFlags(t *testing.T) {
	// Tests use spot-checking of sub and sub-sub commands to ensure that
	// the persistent flags have been propagated.  Note that Namespace is not
	// strictly required for all sub-commands, so the spot-check value of
	// the sub-sub command does not check that Namespace was parsed.
	tests := []struct {
		name              string
		args              []string
		expectedVerbose   bool
		expectedNamespace string
	}{
		{
			name:              "provided as root flags",
			args:              []string{"--verbose", "--namespace=namespace", "deploy"},
			expectedVerbose:   true,
			expectedNamespace: "namespace",
		},
		{
			name:              "provided as sub-command flags",
			args:              []string{"deploy", "--verbose", "--namespace=namespace"},
			expectedVerbose:   true,
			expectedNamespace: "namespace",
		},
		{
			name:              "provided as sub-sub-command flags",
			args:              []string{"repositories", "list", "--verbose", "--namespace=namespace"},
			expectedVerbose:   true,
			expectedNamespace: "", // no sub-sub commands currently parse namespace
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a Function in a temp directory to operate upon
			defer fromTempDir(t)()                    // from a temp dir, deferred cleanup
			cmd := NewCreateCmd(NewClient)            // Create a new Function
			cmd.SetArgs([]string{"--language", "go"}) // providing language (the only required param)
			if err := cmd.Execute(); err != nil {     // fail on any errors
				t.Fatal(err)
			}

			// Assert the persistent variables were propagated to the Client constructor
			// when the command is actually invoked.
			cmd = NewRootCmd("func", Version{}, func(namespace string, verbose bool, options ...fn.Option) (*fn.Client, func()) {
				if namespace != tt.expectedNamespace {
					t.Fatal("namespace not propagated")
				}
				if verbose != tt.expectedVerbose {
					t.Fatal("verbose not propagated")
				}
				return fn.New(), func() {}
			})
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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
			got, err := mergeEnvs(tt.args.envs, tt.args.toUpdate, tt.args.toRemove)
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
	expectedSynopsis := "%v [-v|--verbose] <command> [args]"

	tests := []string{
		"func",    // standalone
		"kn func", // kn plugin
	}

	for _, testName := range tests {
		var (
			cmd = NewRootCmd(testName, Version{}, TestClientFactory)
			out = strings.Builder{}
		)
		cmd.SetOut(&out)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		if cmd.Use != testName {
			t.Fatalf("expected command Use '%v', got '%v'", testName, cmd.Use)
		}
		if !strings.Contains(out.String(), fmt.Sprintf(expectedSynopsis, testName)) {
			t.Logf("Testing '%v'\n", testName)
			t.Log(out.String())
			t.Fatalf("Help text does not include substituted name '%v'", testName)
		}
	}
}

func TestVerbose(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "verbose as version's flag",
			args: []string{"version", "-v"},
			want: "v0.42.0-cafe-1970-01-01\n",
		},
		{
			name: "no verbose",
			args: []string{"version"},
			want: "v0.42.0\n",
		},
		{
			name: "verbose as root's flag",
			args: []string{"--verbose", "version"},
			want: "v0.42.0-cafe-1970-01-01\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			var out bytes.Buffer

			cmd := NewRootCmd("func", Version{
				Date: "1970-01-01",
				Vers: "v0.42.0",
				Hash: "cafe",
			}, TestClientFactory)

			cmd.SetArgs(tt.args)
			cmd.SetOut(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if out.String() != tt.want {
				t.Errorf("expected output: %q but got: %q", tt.want, out.String())
			}
		})
	}
}
