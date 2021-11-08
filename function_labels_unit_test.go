//go:build !integration
// +build !integration

package function

import (
	"os"
	"testing"
)

func Test_validateLabels(t *testing.T) {

	key := "name"
	key2 := "name-two"
	key3 := "prefix.io/name3"
	value := "value"
	value2 := "value2"
	value3 := "value3"

	incorrectKey := ",foo"
	incorrectKey2 := ":foo"
	incorrectValue := ":foo"

	valueLocalEnv := "{{ env:MY_ENV }}"
	valueLocalEnv2 := "{{ env:MY_ENV2 }}"
	valueLocalEnv3 := "{{env:MY_ENV3}}"
	valueLocalEnvIncorrect := "{{ envs:MY_ENV }}"
	valueLocalEnvIncorrect2 := "{{ MY_ENV }}"
	valueLocalEnvIncorrect3 := "{{env:MY_ENV}}foo"

	os.Setenv("BAD_EXAMPLE", ":invalid")
	valueLocalEnvIncorrect4 := "{{env:BAD_EXAMPLE}}"

	os.Setenv("GOOD_EXAMPLE", "valid")
	valueLocalEnv4 := "{{env:GOOD_EXAMPLE}}"

	tests := []struct {
		key    string
		labels []Label
		errs   int
	}{
		{
			"correct entry - single label with value",
			[]Label{
				{
					Key:   &key,
					Value: &value,
				},
			},
			0,
		},
		{
			"correct entry - prefixed label with value",
			[]Label{
				{
					Key:   &key3,
					Value: &value3,
				},
			},
			0,
		}, {
			"incorrect entry - missing key",
			[]Label{
				{
					Value: &value,
				},
			},
			1,
		}, {
			"incorrect entry - missing multiple keys",
			[]Label{
				{
					Value: &value,
				},
				{
					Value: &value2,
				},
			},
			2,
		},
		{
			"incorrect entry - invalid key",
			[]Label{
				{
					Key:   &incorrectKey,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid key2",
			[]Label{
				{
					Key:   &incorrectKey2,
					Value: &value,
				},
			},
			1,
		},
		{
			"incorrect entry - invalid value",
			[]Label{
				{
					Key:   &key,
					Value: &incorrectValue,
				},
			},
			1,
		},
		{
			"correct entry - multiple labels with value",
			[]Label{
				{
					Key:   &key,
					Value: &value,
				},
				{
					Key:   &key2,
					Value: &value2,
				},
			},
			0,
		},
		{
			"correct entry - missing value - multiple labels",
			[]Label{
				{
					Key: &key,
				},
				{
					Key: &key2,
				},
			},
			0,
		},
		{
			"correct entry - single label with value from local env",
			[]Label{
				{
					Key:   &key,
					Value: &valueLocalEnv,
				},
			},
			0,
		},
		{
			"correct entry - multiple labels with values from Local env",
			[]Label{
				{
					Key:   &key,
					Value: &valueLocalEnv,
				},
				{
					Key:   &key,
					Value: &valueLocalEnv2,
				},
				{
					Key:   &key,
					Value: &valueLocalEnv3,
				},
			},
			0,
		},
		{
			"incorrect entry - multiple labels with values from Local env",
			[]Label{
				{
					Key:   &key,
					Value: &valueLocalEnv,
				},
				{
					Key:   &key,
					Value: &valueLocalEnvIncorrect,
				},
				{
					Key:   &key,
					Value: &valueLocalEnvIncorrect2,
				},
				{
					Key:   &key,
					Value: &valueLocalEnvIncorrect3,
				},
			},
			3,
		},
		{
			"correct entry - good environment variable value",
			[]Label{
				{
					Key:   &key,
					Value: &valueLocalEnv4,
				},
			},
			0,
		}, {
			"incorrect entry - bad environment variable value",
			[]Label{
				{
					Key:   &key,
					Value: &valueLocalEnvIncorrect4,
				},
			},
			1,
		},
		{
			"correct entry - all combinations",
			[]Label{
				{
					Key:   &key,
					Value: &value,
				},
				{
					Key:   &key2,
					Value: &value2,
				},
				{
					Key:   &key3,
					Value: &value3,
				},
				{
					Key:   &key,
					Value: &valueLocalEnv,
				},
				{
					Key:   &key,
					Value: &valueLocalEnv2,
				},
				{
					Key:   &key,
					Value: &valueLocalEnv3,
				},
			},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := ValidateLabels(tt.labels); len(got) != tt.errs {
				t.Errorf("validateLabels() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
