// +build !integration

package function

import (
	"testing"

	"knative.dev/pkg/ptr"
)

func Test_validateVolumes(t *testing.T) {

	secret := "secret"
	path := "path"
	secret2 := "secret2"
	path2 := "path2"
	cm := "configMap"

	tests := []struct {
		name    string
		volumes Volumes
		errs    int
	}{
		{
			"correct entry - single volume with secret",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
			},
			0,
		},
		{
			"correct entry - single volume with configmap",
			Volumes{
				Volume{
					ConfigMap: &cm,
					Path:      &path,
				},
			},
			0,
		},
		{
			"correct entry - multiple volumes with secrets",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
				Volume{
					Secret: &secret2,
					Path:   &path2,
				},
			},
			0,
		},
		{
			"correct entry - multiple volumes with both secret and configMap",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
				Volume{
					ConfigMap: &cm,
					Path:      &path2,
				},
			},
			0,
		},
		{
			"missing secret/configMap - single volume",
			Volumes{
				Volume{
					Path: &path,
				},
			},
			1,
		},
		{
			"missing path - single volume with secret",
			Volumes{
				Volume{
					Secret: &secret,
				},
			},
			1,
		},
		{
			"missing path - single volume with configMap",
			Volumes{
				Volume{
					ConfigMap: &cm,
				},
			},
			1,
		},
		{
			"missing secret/configMap and path - single volume",
			Volumes{
				Volume{},
			},
			1,
		},
		{
			"missing secret/configMap in one volume - multiple volumes",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
				Volume{
					Path: &path2,
				},
			},
			1,
		},
		{
			"missing secret/configMap and path in two different volumes - multiple volumes",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
				Volume{
					Secret: &secret,
				},
				Volume{
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

func Test_validateEnvs(t *testing.T) {

	name := "name"
	name2 := "name2"
	value := "value"
	value2 := "value2"

	incorrectName := ",foo"
	incorrectName2 := ":foo"

	valueLocalEnv := "{{ env.MY_ENV }}"
	valueLocalEnv2 := "{{ env.MY_ENV2 }}"
	valueLocalEnv3 := "{{env.MY_ENV3}}"
	valueLocalEnvIncorrect := "{{ envs.MY_ENV }}"
	valueLocalEnvIncorrect2 := "{{ MY_ENV }}"
	valueLocalEnvIncorrect3 := "{{env.MY_ENV}}foo"

	valueSecretKey := "{{ secret.mysecret.key }}"
	valueSecretKey2 := "{{secret.my-secret.key }}"
	valueSecretKey3 := "{{secret.my-secret.key2}}"
	valueSecretKeyIncorrect := "{{ secret.my-secret.key.key }}"
	valueSecretKeyIncorrect2 := "{{ my-secret.key }}"
	valueSecretKeyIncorrect3 := "{{ secret.my-secret.key }}foo"
	valueConfigMapKey := "{{ configMap.myconfigmap.key }}"

	valueSecret := "{{ secret.my-secret }}"
	valueSecret2 := "{{ secret.mysecret }}"
	valueSecret3 := "{{ secret.mysecret}}"
	valueSecretIncorrect := "{{ my-secret }}"
	valueSecretIncorrect2 := "my-secret"
	valueSecretIncorrect3 := "{{ secret.my-secret }}foo"
	valueConfigMap := "{{ configMap.myconfigmap }}"

	tests := []struct {
		name string
		envs Envs
		errs int
	}{
		{
			"correct entry - single env with value",
			Envs{
				Env{
					Name:  &name,
					Value: &value,
				},
			},
			0,
		},
		{
			"incorrect entry - missing value",
			Envs{
				Env{
					Name: &name,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name",
			Envs{
				Env{
					Name:  &incorrectName,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name2",
			Envs{
				Env{
					Name:  &incorrectName2,
					Value: &value,
				},
			},
			1,
		},
		{
			"correct entry - multiple envs with value",
			Envs{
				Env{
					Name:  &name,
					Value: &value,
				},
				Env{
					Name:  &name2,
					Value: &value2,
				},
			},
			0,
		},
		{
			"incorrect entry - mmissing value - multiple envs",
			Envs{
				Env{
					Name: &name,
				},
				Env{
					Name: &name2,
				},
			},
			2,
		},
		{
			"correct entry - single env with value Local env",
			Envs{
				Env{
					Name:  &name,
					Value: &valueLocalEnv,
				},
			},
			0,
		},
		{
			"correct entry - multiple envs with value Local env",
			Envs{
				Env{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnv2,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnv3,
				},
			},
			0,
		},
		{
			"incorrect entry - multiple envs with value Local env",
			Envs{
				Env{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnvIncorrect,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnvIncorrect2,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnvIncorrect3,
				},
			},
			3,
		},
		{
			"correct entry - single secret with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueSecretKey,
				},
			},
			0,
		},
		{
			"correct entry - single configMap with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
		{
			"correct entry - multiple secrets with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueSecretKey,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKey2,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKey3,
				},
			},
			0,
		},
		{
			"correct entry - both secret and configmap with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueSecretKey,
				},
				Env{
					Name:  &name,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
		{
			"incorrect entry - single secret with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueSecretKeyIncorrect,
				},
			},
			1,
		},
		{
			"incorrect entry - mutliple secrets with key",
			Envs{
				Env{
					Name:  &name,
					Value: &valueSecretKey,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKeyIncorrect,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKeyIncorrect2,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKeyIncorrect3,
				},
			},
			3,
		},
		{
			"correct entry - single whole secret",
			Envs{
				Env{
					Value: &valueSecret,
				},
			},
			0,
		},
		{
			"correct entry - single whole configMap",
			Envs{
				Env{
					Value: &valueConfigMap,
				},
			},
			0,
		},
		{
			"correct entry - multiple whole secret",
			Envs{
				Env{
					Value: &valueSecret,
				},
				Env{
					Value: &valueSecret2,
				},
				Env{
					Value: &valueSecret3,
				},
			},
			0,
		},
		{
			"correct entry - both whole secret and configMap",
			Envs{
				Env{
					Value: &valueSecret,
				},
				Env{
					Value: &valueConfigMap,
				},
			},
			0,
		},
		{
			"incorrect entry - single whole secret",
			Envs{
				Env{
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - multiple whole secret",
			Envs{
				Env{
					Value: &valueSecretIncorrect,
				},
				Env{
					Value: &valueSecretIncorrect2,
				},
				Env{
					Value: &valueSecretIncorrect3,
				},
				Env{
					Value: &value,
				},
				Env{
					Value: &valueLocalEnv,
				},
				Env{
					Value: &valueLocalEnv2,
				},
				Env{
					Value: &valueLocalEnv3,
				},
				Env{
					Value: &valueSecret,
				},
			},
			7,
		},
		{
			"correct entry - all combinations",
			Envs{
				Env{
					Name:  &name,
					Value: &value,
				},
				Env{
					Name:  &name2,
					Value: &value2,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnv2,
				},
				Env{
					Name:  &name,
					Value: &valueLocalEnv3,
				},
				Env{
					Value: &valueSecret,
				},
				Env{
					Value: &valueSecret2,
				},
				Env{
					Value: &valueSecret3,
				},
				Env{
					Value: &valueConfigMap,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKey,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKey2,
				},
				Env{
					Name:  &name,
					Value: &valueSecretKey3,
				},
				Env{
					Name:  &name,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateEnvs(tt.envs); len(got) != tt.errs {
				t.Errorf("validateEnvs() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}

}

func Test_validateOptions(t *testing.T) {

	tests := []struct {
		name    string
		options Options
		errs    int
	}{
		{
			"correct 'scale.metric' - concurrency",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("concurrency"),
				},
			},
			0,
		},
		{
			"correct 'scale.metric' - rps",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("rps"),
				},
			},
			0,
		},
		{
			"incorrect 'scale.metric'",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("foo"),
				},
			},
			1,
		},
		{
			"correct 'scale.min'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(1),
				},
			},
			0,
		},
		{
			"correct 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Max: ptr.Int64(10),
				},
			},
			0,
		},
		{
			"correct  'scale.min' & 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(0),
					Max: ptr.Int64(10),
				},
			},
			0,
		},
		{
			"incorrect  'scale.min' & 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(100),
					Max: ptr.Int64(10),
				},
			},
			1,
		},
		{
			"incorrect 'scale.min' - negative value",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(-10),
				},
			},
			1,
		},
		{
			"incorrect 'scale.max' - negative value",
			Options{
				Scale: &ScaleOptions{
					Max: ptr.Int64(-10),
				},
			},
			1,
		},
		{
			"correct 'scale.target'",
			Options{
				Scale: &ScaleOptions{
					Target: ptr.Float64(50),
				},
			},
			0,
		},
		{
			"incorrect 'scale.target'",
			Options{
				Scale: &ScaleOptions{
					Target: ptr.Float64(0),
				},
			},
			1,
		},
		{
			"correct 'scale.utilization'",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(50),
				},
			},
			0,
		},
		{
			"incorrect 'scale.utilization' - < 1",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(0),
				},
			},
			1,
		},
		{
			"incorrect 'scale.utilization' - > 100",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(110),
				},
			},
			1,
		},
		{
			"correct all options",
			Options{
				Scale: &ScaleOptions{
					Min:         ptr.Int64(0),
					Max:         ptr.Int64(10),
					Metric:      ptr.String("concurrency"),
					Target:      ptr.Float64(40.5),
					Utilization: ptr.Float64(35.5),
				},
			},
			0,
		},
		{
			"incorrect all options",
			Options{
				Scale: &ScaleOptions{
					Min:         ptr.Int64(-1),
					Max:         ptr.Int64(-1),
					Metric:      ptr.String("foo"),
					Target:      ptr.Float64(-1),
					Utilization: ptr.Float64(110),
				},
			},
			5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateOptions(tt.options); len(got) != tt.errs {
				t.Errorf("validateOptions() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}

}
