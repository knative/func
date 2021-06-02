// +build !integration

package function

import (
	"testing"
)

func Test_validateVolumes(t *testing.T) {

	secret := "secret"
	path := "path"
	secret2 := "secret2"
	path2 := "path2"

	tests := []struct {
		name    string
		volumes Volumes
		errs    int
	}{
		{
			"correct entry - single volume",
			Volumes{
				Volume{
					Secret: &secret,
					Path:   &path,
				},
			},
			0,
		},
		{
			"correct entry - multiple volumes",
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
			"missing secret - single volume",
			Volumes{
				Volume{
					Path: &path,
				},
			},
			1,
		},
		{
			"missing secret - single volume",
			Volumes{
				Volume{
					Secret: &secret,
				},
			},
			1,
		},
		{
			"missing secret and path - single volume",
			Volumes{
				Volume{},
			},
			1,
		},
		{
			"missing secret in one volume - multiple volumes",
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
			"missing secret and path in two different volumes - multiple volumes",
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

	valueSecret := "{{ secret.my-secret }}"
	valueSecret2 := "{{ secret.mysecret }}"
	valueSecret3 := "{{ secret.mysecret}}"
	valueSecretIncorrect := "{{ my-secret }}"
	valueSecretIncorrect2 := "my-secret"
	valueSecretIncorrect3 := "{{ secret.my-secret }}foo"

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
			"correct entry - multiple whole secret2",
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
			"incorrect entry - single whole secret",
			Envs{
				Env{
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - multiple whole secret2",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateEnvs(tt.envs); len(got) != tt.errs {
				t.Errorf("validateEnvs() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}

}
