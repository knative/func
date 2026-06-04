package functions

import (
	"fmt"
	"testing"
)

func Test_validateBuildEnvs(t *testing.T) {

	name := "name"
	name2 := "name2"
	name3 := "name3"
	name4 := "name4"
	value := "value"
	value2 := "value2"

	incorrectName := ",foo"
	incorrectName2 := ":foo"

	valueLocalEnv := "{{ env:MY_ENV }}"
	valueLocalEnv2 := "{{ env:MY_ENV2 }}"
	valueLocalEnv3 := "{{env:MY_ENV3}}"
	valueLocalEnvIncorrect := "{{ envs:MY_ENV }}"
	valueLocalEnvIncorrect2 := "{{ MY_ENV }}"
	valueLocalEnvIncorrect3 := "{{env:MY_ENV}}foo"

	tests := []struct {
		name string
		envs []Env
		errs int
	}{
		{
			"correct entry - single env with value",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
			},
			0,
		},
		{
			"incorrect entry - missing value",
			[]Env{
				{
					Name: &name,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name",
			[]Env{
				{
					Name:  &incorrectName,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name2",
			[]Env{
				{
					Name:  &incorrectName2,
					Value: &value,
				},
			},
			1,
		},
		{
			"correct entry - multiple envs with value",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
				{
					Name:  &name2,
					Value: &value2,
				},
			},
			0,
		},
		{
			"incorrect entry - missing value - multiple envs",
			[]Env{
				{
					Name: &name,
				},
				{
					Name: &name2,
				},
			},
			2,
		},
		{
			"correct entry - single env with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
			},
			0,
		},
		{
			"correct entry - multiple envs with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				{
					Name:  &name2,
					Value: &valueLocalEnv2,
				},
				{
					Name:  &name3,
					Value: &valueLocalEnv3,
				},
			},
			0,
		},
		{
			"incorrect entry - multiple envs with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				{
					Name:  &name2,
					Value: &valueLocalEnvIncorrect,
				},
				{
					Name:  &name3,
					Value: &valueLocalEnvIncorrect2,
				},
				{
					Name:  &name4,
					Value: &valueLocalEnvIncorrect3,
				},
			},
			3,
		},
		{
			"incorrect entry - duplicate env names",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
				{
					Name:  &name,
					Value: &value2,
				},
			},
			1,
		},
		{
			"incorrect entry - missing name",
			[]Env{
				{
					Value: &value,
				},
			},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateBuildEnvs(tt.envs); len(got) != tt.errs {
				t.Errorf("ValidateBuildEnvs() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}

func Test_validateEnvs(t *testing.T) {

	name := "name"
	name2 := "name2"
	name3 := "name3"
	name4 := "name4"
	name5 := "name5"
	name6 := "name6"
	name7 := "name7"
	name8 := "name8"
	name9 := "name9"
	value := "value"
	value2 := "value2"

	incorrectName := ",foo"
	incorrectName2 := ":foo"

	valueLocalEnv := "{{ env:MY_ENV }}"
	valueLocalEnv2 := "{{ env:MY_ENV2 }}"
	valueLocalEnv3 := "{{env:MY_ENV3}}"
	valueLocalEnvIncorrect := "{{ envs:MY_ENV }}"
	valueLocalEnvIncorrect2 := "{{ MY_ENV }}"
	valueLocalEnvIncorrect3 := "{{env:MY_ENV}}foo"

	valueSecretKey := "{{ secret:mysecret:key }}"
	valueSecretKey2 := "{{secret:my-secret:key }}"
	valueSecretKey3 := "{{secret:my-secret:key2}}"
	valueSecretKey4 := "{{secret:my-secret:key-2}}"
	valueSecretKey5 := "{{secret:my-secret:key.2}}"
	valueSecretKey6 := "{{secret:my-secret:key_2}}"
	valueSecretKey7 := "{{secret:my-secret:key_2-1}}"
	valueSecretKey8 := "{{secret:my-secret:key_2-1.3}}"
	valueSecretKeyIncorrect := "{{ secret:my-secret:key,key }}"
	valueSecretKeyIncorrect2 := "{{ my-secret:key }}"
	valueSecretKeyIncorrect3 := "{{ secret:my-secret:key }}foo"
	valueConfigMapKey := "{{ configMap:myconfigmap:key }}"
	valueConfigMapKey2 := "{{ configMap:myconfigmap:key }}"
	valueConfigMapKey3 := "{{ configMap:myconfigmap:key2 }}"
	valueConfigMapKey4 := "{{ configMap:myconfigmap:key-2 }}"
	valueConfigMapKey5 := "{{ configMap:myconfigmap:key.2 }}"
	valueConfigMapKey6 := "{{ configMap:myconfigmap:key_2 }}"
	valueConfigMapKey7 := "{{ configMap:myconfigmap:key_2-1 }}"
	valueConfigMapKey8 := "{{ configMap:myconfigmap:key_2.1 }}"

	valueSecret := "{{ secret:my-secret }}"
	valueSecret2 := "{{ secret:mysecret }}"
	valueSecret3 := "{{ secret:mysecret}}"
	valueSecretIncorrect := "{{ my-secret }}"
	valueSecretIncorrect2 := "my-secret"
	valueSecretIncorrect3 := "{{ secret:my-secret }}foo"
	valueConfigMap := "{{ configMap:myconfigmap }}"

	tests := []struct {
		name string
		envs []Env
		errs int
	}{
		{
			"correct entry - single env with value",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
			},
			0,
		},
		{
			"incorrect entry - missing value",
			[]Env{
				{
					Name: &name,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name",
			[]Env{
				{
					Name:  &incorrectName,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid name2",
			[]Env{
				{
					Name:  &incorrectName2,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - no name no value",
			[]Env{
				{},
			},
			1,
		},
		{
			"correct entry - multiple envs with value",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
				{
					Name:  &name2,
					Value: &value2,
				},
			},
			0,
		},
		{
			"incorrect entry - missing value - multiple envs",
			[]Env{
				{
					Name: &name,
				},
				{
					Name: &name2,
				},
			},
			2,
		},
		{
			"correct entry - single env with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
			},
			0,
		},
		{
			"correct entry - multiple envs with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				{
					Name:  &name2,
					Value: &valueLocalEnv2,
				},
				{
					Name:  &name3,
					Value: &valueLocalEnv3,
				},
			},
			0,
		},
		{
			"incorrect entry - multiple envs with value Local env",
			[]Env{
				{
					Name:  &name,
					Value: &valueLocalEnv,
				},
				{
					Name:  &name2,
					Value: &valueLocalEnvIncorrect,
				},
				{
					Name:  &name3,
					Value: &valueLocalEnvIncorrect2,
				},
				{
					Name:  &name4,
					Value: &valueLocalEnvIncorrect3,
				},
			},
			3,
		},
		{
			"correct entry - single secret with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueSecretKey,
				},
			},
			0,
		},
		{
			"correct entry - single configMap with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
		{
			"correct entry - multiple configMaps with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueConfigMapKey,
				},
				{
					Name:  &name2,
					Value: &valueConfigMapKey2,
				},
				{
					Name:  &name3,
					Value: &valueConfigMapKey3,
				},
				{
					Name:  &name4,
					Value: &valueConfigMapKey4,
				},
				{
					Name:  &name5,
					Value: &valueConfigMapKey5,
				},
				{
					Name:  &name6,
					Value: &valueConfigMapKey6,
				},
				{
					Name:  &name7,
					Value: &valueConfigMapKey7,
				},
				{
					Name:  &name8,
					Value: &valueConfigMapKey8,
				},
			},
			0,
		},
		{
			"correct entry - multiple secrets with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueSecretKey,
				},
				{
					Name:  &name2,
					Value: &valueSecretKey2,
				},
				{
					Name:  &name3,
					Value: &valueSecretKey3,
				},
				{
					Name:  &name4,
					Value: &valueSecretKey4,
				},
				{
					Name:  &name5,
					Value: &valueSecretKey5,
				},
				{
					Name:  &name6,
					Value: &valueSecretKey6,
				},
				{
					Name:  &name7,
					Value: &valueSecretKey7,
				},
				{
					Name:  &name8,
					Value: &valueSecretKey8,
				},
			},
			0,
		},
		{
			"correct entry - both secret and configmap with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueSecretKey,
				},
				{
					Name:  &name2,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
		{
			"incorrect entry - single secret with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueSecretKeyIncorrect,
				},
			},
			1,
		},
		{
			"incorrect entry - multiple secrets with key",
			[]Env{
				{
					Name:  &name,
					Value: &valueSecretKey,
				},
				{
					Name:  &name2,
					Value: &valueSecretKeyIncorrect,
				},
				{
					Name:  &name3,
					Value: &valueSecretKeyIncorrect2,
				},
				{
					Name:  &name4,
					Value: &valueSecretKeyIncorrect3,
				},
			},
			3,
		},
		{
			"correct entry - single whole secret",
			[]Env{
				{
					Value: &valueSecret,
				},
			},
			0,
		},
		{
			"correct entry - single whole configMap",
			[]Env{
				{
					Value: &valueConfigMap,
				},
			},
			0,
		},
		{
			"correct entry - multiple whole secret",
			[]Env{
				{
					Value: &valueSecret,
				},
				{
					Value: &valueSecret2,
				},
				{
					Value: &valueSecret3,
				},
			},
			0,
		},
		{
			"correct entry - both whole secret and configMap",
			[]Env{
				{
					Value: &valueSecret,
				},
				{
					Value: &valueConfigMap,
				},
			},
			0,
		},
		{
			"incorrect entry - single whole secret",
			[]Env{
				{
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - multiple whole secret",
			[]Env{
				{
					Value: &valueSecretIncorrect,
				},
				{
					Value: &valueSecretIncorrect2,
				},
				{
					Value: &valueSecretIncorrect3,
				},
				{
					Value: &value,
				},
				{
					Value: &valueLocalEnv,
				},
				{
					Value: &valueLocalEnv2,
				},
				{
					Value: &valueLocalEnv3,
				},
				{
					Value: &valueSecret,
				},
			},
			7,
		},
		{
			"correct entry - all combinations",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
				{
					Name:  &name2,
					Value: &value2,
				},
				{
					Name:  &name3,
					Value: &valueLocalEnv,
				},
				{
					Name:  &name4,
					Value: &valueLocalEnv2,
				},
				{
					Name:  &name5,
					Value: &valueLocalEnv3,
				},
				{
					Value: &valueSecret,
				},
				{
					Value: &valueSecret2,
				},
				{
					Value: &valueSecret3,
				},
				{
					Value: &valueConfigMap,
				},
				{
					Name:  &name6,
					Value: &valueSecretKey,
				},
				{
					Name:  &name7,
					Value: &valueSecretKey2,
				},
				{
					Name:  &name8,
					Value: &valueSecretKey3,
				},
				{
					Name:  &name9,
					Value: &valueConfigMapKey,
				},
			},
			0,
		},
		{
			"incorrect entry - duplicate env names",
			[]Env{
				{
					Name:  &name,
					Value: &value,
				},
				{
					Name:  &name,
					Value: &value2,
				},
			},
			1,
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

func Test_KeyValuePair(t *testing.T) {
	name := "name"
	value := "value"

	tests := []struct {
		name string
		env  Env
		want string
	}{
		{
			"name & value",
			Env{

				Name:  &name,
				Value: &value,
			},
			fmt.Sprintf("%s=%s", name, value),
		},
		{
			"name only",
			Env{

				Name: &name,
			},
			fmt.Sprintf("%s=", name),
		},
		{
			"value only",
			Env{

				Value: &value,
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.KeyValuePair(); got != tt.want {
				t.Errorf("KeyValuePair() for env = %v\n got %q errors but want %q", tt.env, got, tt.want)
			}
		})
	}
}

func Test_validateEnvString(t *testing.T) {

	name := "name"

	value := "value"

	valueLocalEnv := "{{ env:MY_ENV }}"
	valueConfigMapKey := "{{ configMap:myconfigmap:key }}"

	valueSecretKey := "{{ secret:mysecret:key }}"

	tests := []struct {
		key  string
		env  Env
		want string
	}{
		{
			"env with name and value",
			Env{
				Name:  &name,
				Value: &value,
			},
			"Env \"name\" with value \"value\"",
		},
		{
			"env with name and value from local env",
			Env{
				Name:  &name,
				Value: &valueLocalEnv,
			},
			"Env \"name\" with value set from local env variable \"MY_ENV\"",
		},

		{
			"env with name and value from configmap",
			Env{
				Name:  &name,
				Value: &valueConfigMapKey,
			},
			"Env \"name\" with value set from key \"key\" from ConfigMap \"myconfigmap\"",
		},
		{
			"env with name and value from secret",
			Env{
				Name:  &name,
				Value: &valueSecretKey,
			},
			"Env \"name\" with value set from key \"key\" from Secret \"mysecret\"",
		},
		{
			//@TODO: is this an edge-case that we are not covering?
			"env with name and no value",
			Env{
				Name: &name,
			},
			"",
		},
		{
			"env with no name and no value",
			Env{},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if tt.env.String() != tt.want {
				t.Errorf("validateEnvString() = \n got %v but expected %v", tt.env.String(), tt.want)
			}
		})
	}
}
