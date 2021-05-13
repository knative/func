package cmd

import (
	"reflect"
	"testing"
)

func Test_mergeEnvMaps(t *testing.T) {
	type args struct {
		dest map[string]string
		src  map[string]string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			"add new var",
			args{
				map[string]string{"A": "1"},
				map[string]string{"B": "2"},
			},
			map[string]string{"A": "1", "B": "2"},
		},
		{
			"update var",
			args{
				map[string]string{"A": "1"},
				map[string]string{"A": "2"},
			},
			map[string]string{"A": "2"},
		},
		{
			"remove var",
			args{
				map[string]string{"A": "1"},
				map[string]string{"A-": ""},
			},
			map[string]string{"A-": ""},
		},
		{
			"re-add var",
			args{
				map[string]string{"A-": ""},
				map[string]string{"A": "1"},
			},
			map[string]string{"A": "1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeEnvMaps(tt.args.dest, tt.args.src); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeEnvMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}
