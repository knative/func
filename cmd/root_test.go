package cmd

import (
	"fmt"
	"reflect"
	"testing"

	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
)

func Test_mergeEnvMaps(t *testing.T) {

	a := "A"
	b := "B"
	v1 := "x"
	v2 := "y"

	type args struct {
		envs     fn.Envs
		toUpdate *util.OrderedMap
		toRemove []string
	}
	tests := []struct {
		name string
		args args
		want fn.Envs
	}{
		{
			"add new var to empty list",
			args{
				fn.Envs{},
				util.NewOrderedMapWithKVStrings([][]string{{a, v1}}),
				[]string{},
			},
			fn.Envs{fn.Env{Name: &a, Value: &v1}},
		},
		{
			"add new var",
			args{
				fn.Envs{fn.Env{Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v1}}),
				[]string{},
			},
			fn.Envs{fn.Env{Name: &b, Value: &v2}, fn.Env{Name: &a, Value: &v1}},
		},
		{
			"update var",
			args{
				fn.Envs{fn.Env{Name: &a, Value: &v1}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}}),
				[]string{},
			},
			fn.Envs{fn.Env{Name: &a, Value: &v2}},
		},
		{
			"update multiple vars",
			args{
				fn.Envs{fn.Env{Name: &a, Value: &v1}, fn.Env{Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}, {b, v1}}),
				[]string{},
			},
			fn.Envs{fn.Env{Name: &a, Value: &v2}, fn.Env{Name: &b, Value: &v1}},
		},
		{
			"remove var",
			args{
				fn.Envs{fn.Env{Name: &a, Value: &v1}},
				util.NewOrderedMap(),
				[]string{a},
			},
			fn.Envs{},
		},
		{
			"remove multiple vars",
			args{
				fn.Envs{fn.Env{Name: &a, Value: &v1}, fn.Env{Name: &b, Value: &v2}},
				util.NewOrderedMap(),
				[]string{a, b},
			},
			fn.Envs{},
		},
		{
			"update and remove vars",
			args{
				fn.Envs{fn.Env{Name: &a, Value: &v1}, fn.Env{Name: &b, Value: &v2}},
				util.NewOrderedMapWithKVStrings([][]string{{a, v2}}),
				[]string{b},
			},
			fn.Envs{fn.Env{Name: &a, Value: &v2}},
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

func Test_CMDParameterized(t *testing.T) {

	if root.Use != "func" {
		t.Fatalf("default command use should be \"func\".")
	}

	if root.Example != replaceNameInTemplate("func", "example") {
		t.Fatalf("default command example should assume \"func\" as executable name.")
	}

	if NewRootCmd().Use != "kn func" {
		t.Fatalf("plugin command use should be \"kn func\".")
	}

	if NewRootCmd().Example != replaceNameInTemplate("kn func", "example") {
		t.Fatalf("plugin command example should assume \"kn func\" as executable name.")
	}
}
