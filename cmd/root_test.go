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

func TestRoot_CMDParameterized(t *testing.T) {
	expectedSynopsis := "%v [-v|--verbose] <command> [args]"

	tests := []string{
		"func",
		"kn func",
	}

	for _, test := range tests {
		var (
			cfg    = RootCommandConfig{Name: test}
			cmd, _ = NewRootCmd(cfg)
			out    = strings.Builder{}
		)
		cmd.SetOut(&out)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		if cmd.Use != cfg.Name {
			t.Fatalf("expected command Use '%v', got '%v'", cfg.Name, cmd.Use)
		}
		if !strings.Contains(out.String(), fmt.Sprintf(expectedSynopsis, cfg.Name)) {
			t.Logf("Testing '%v'\n", test)
			t.Log(out.String())
			t.Fatalf("Help text does not include substituted name '%v'", cfg.Name)
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
		{
			name: "version not as sub-command",
			args: []string{"--version"},
			want: "v0.42.0\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			var out bytes.Buffer

			cmd, err := NewRootCmd(RootCommandConfig{
				Name:    "func",
				Date:    "1970-01-01",
				Version: "v0.42.0",
				Hash:    "cafe",
			})
			if err != nil {
				t.Fatal(err)
			}

			cmd.SetArgs(tt.args)
			cmd.SetOut(&out)
			err = cmd.Execute()
			if err != nil {
				t.Fatal(err)
			}

			if out.String() != tt.want {
				t.Errorf("expected output: %q but got: %q", tt.want, out.String())
			}
		})
	}
}
