package functions

import (
	"testing"
)

func Test_validateVolumes(t *testing.T) {

	path := "path"
	path2 := "path2"
	path3 := "path3"
	path4 := "path4"
	secret := "secret"
	secret2 := "secret2"
	cm := "configMap"
	pvcName := "pvc"
	pvc := &PersistentVolumeClaim{
		ClaimName: &pvcName,
	}
	emptyDir := &EmptyDir{}

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
			"correct entry - multiple volumes with secret, configMap, persistentVolumeClaim, and emptyDir",
			[]Volume{
				{
					Secret: &secret,
					Path:   &path,
				},
				{
					ConfigMap: &cm,
					Path:      &path2,
				},
				{
					PersistentVolumeClaim: pvc,
					Path:                  &path3,
				},
				{
					EmptyDir: emptyDir,
					Path:     &path4,
				},
			},
			0,
		},
		{
			"missing volume type - single volume",
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
			"missing volume type and path - single volume",
			[]Volume{
				{},
			},
			2,
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
			"missing volume type and path in two different volumes - multiple volumes",
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
		{
			"multiple volume types - single volume with path",
			[]Volume{
				{
					Secret:    &secret,
					ConfigMap: &cm,
					Path:      &path,
				},
			},
			1,
		},
		{
			"multiple volume types, missing path - single volume",
			[]Volume{
				{
					Secret:    &secret,
					ConfigMap: &cm,
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
	pvcName := "pvc"
	pvc := &PersistentVolumeClaim{
		ClaimName: &pvcName,
	}

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
			"Secret \"secret\" at path: \"path\"",
		},
		{
			"volume with configMap and path",
			Volume{
				ConfigMap: &cm,
				Path:      &path,
			},
			"ConfigMap \"configMap\" at path: \"path\"",
		},
		{
			"volume with persistentVolumeClaim and path",
			Volume{
				PersistentVolumeClaim: pvc,
				Path:                  &path,
			},
			"PersistentVolumeClaim \"pvc\" at path: \"path\"",
		},
		{
			"volume with emptyDir and path",
			Volume{
				EmptyDir: &EmptyDir{},
				Path:     &path,
			},
			"EmptyDir at path: \"path\"",
		},
		{
			"volume with no volume type but with path",
			Volume{
				Path: &path,
			},
			"No volume type at path: \"path\"",
		},
		{
			"volume with secret but no path",
			Volume{
				Secret: &secret,
			},
			"Secret \"secret\"",
		},
		{
			"volume with no volume type or path",
			Volume{},
			"No volume type",
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
