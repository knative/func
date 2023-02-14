//go:build !integration
// +build !integration

package functions

import (
	"testing"
)

func Test_validateVolumes(t *testing.T) {

	secret := "secret"
	path := "path"
	secret2 := "secret2"
	path2 := "path2"
	cm := "configMap"

	tests := []struct {
		name    string
		volumes []Volume
		errs    int
	}{
		{
			"incorrect entry - no secret or configMap only path",
			[]Volume{
				{
					Path: &path,
				},
			},
			1,
		},
		{
			"correct entry - single volume with secret",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
			},
			0,
		},
		{
			"correct entry - single volume with configmap",
			[]Volume{
				{
					ConfigMap: &cm,
					Path:      &path,
				},
			},
			0,
		},
		{
			"correct entry - multiple volumes with secrets",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
				{
					Secret: &secret2,
					Path:   &path2,
				},
			},
			0,
		},
		{
			"correct entry - multiple volumes with both secret and configMap",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
				{
					ConfigMap: &cm,
					Path:      &path2,
				},
			},
			0,
		},
		{
			"missing secret/configMap - single volume",
			[]Volume{
				{
					Path: &path,
				},
			},
			1,
		},
		{
			"missing path - single volume with secret",
			[]Volume{
				{
					Secret: &secret,
				},
			},
			1,
		},
		{
			"missing path - single volume with configMap",
			[]Volume{
				{
					ConfigMap: &cm,
				},
			},
			1,
		},
		{
			"missing secret/configMap and path - single volume",
			[]Volume{
				{},
			},
			1,
		},
		{
			"missing secret/configMap in one volume - multiple volumes",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
				{
					Path: &path2,
				},
			},
			1,
		},
		{
			"missing secret/configMap and path in two different volumes - multiple volumes",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
				{
					Secret: &secret,
				},
				{
					Path: &path2,
				},
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateVolumes(tt.volumes); len(got) != tt.errs {
				t.Errorf("validateVolumes() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}

func Test_validateVolumesString(t *testing.T) {

	secret := "secret"
	path := "path"

	cm := "configMap"

	tests := []struct {
		key    string
		volume Volume
		want   string
	}{
		{
			"volume with secret and path",
			Volume{
				Secret: &secret,
				Path:   &path,
			},
			"Secret \"secret\" mounted at path: \"path\"",
		},
		{
			"volume with configMap and path",
			Volume{
				ConfigMap: &cm,
				Path:      &path,
			},
			"ConfigMap \"configMap\" mounted at path: \"path\"",
		},
		{
			//@TODO:this is and edge case that we are not covering
			"volume with no configMap and no secret but with path",
			Volume{
				Path: &path,
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if tt.volume.String() != tt.want {
				t.Errorf("validateVolumeString() = \n got %v but expected %v", tt.volume.String(), tt.want)
			}
		})
	}
}
