//go:build !integration
// +build !integration

package function

import (
	"reflect"
	"testing"

	fnlabels "knative.dev/kn-plugin-func/k8s/labels"
)

func TestFunction_ImageWithDigest(t *testing.T) {
	type fields struct {
		Image       string
		ImageDigest string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "Full path with port",
			fields: fields{Image: "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar", ImageDigest: "42"},
			want:   "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@42",
		},
		{
			name:   "Path with namespace",
			fields: fields{Image: "johndoe/bar", ImageDigest: "42"},
			want:   "johndoe/bar@42",
		},
		{
			name:   "Just image name",
			fields: fields{Image: "bar:latest", ImageDigest: "42"},
			want:   "bar@42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Function{
				Image:       tt.fields.Image,
				ImageDigest: tt.fields.ImageDigest,
			}
			if got := f.ImageWithDigest(); got != tt.want {
				t.Errorf("ImageWithDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFunction_ImageName ensures that the full image name is
// returned for a Function, based on the Function's Registry and Name,
// including utilizing the DefaultRegistry if the Function's defined
// registry is a single token (just the namespace).
func TestFunction_ImageName(t *testing.T) {
	var (
		f   Function
		got string
		err error
	)
	tests := []struct {
		registry      string
		name          string
		expectedImage string
		expectError   bool
	}{
		{"alice", "myfunc", DefaultRegistry + "/alice/myfunc:latest", false},
		{"quay.io/alice", "myfunc", "quay.io/alice/myfunc:latest", false},
		{"docker.io/alice", "myfunc", "docker.io/alice/myfunc:latest", false},
		{"docker.io/alice/sub", "myfunc", "docker.io/alice/sub/myfunc:latest", false},
		{"alice", "", "", true},
		{"", "myfunc", "", true},
	}
	for _, test := range tests {
		f = Function{Registry: test.registry, Name: test.name}
		got, err = f.ImageName()
		if test.expectError && err == nil {
			t.Errorf("registry '%v' and name '%v' did not yield the expected error",
				test.registry, test.name)
		}
		if got != test.expectedImage {
			t.Errorf("expected registry '%v' name '%v' to yield image '%v', got '%v'",
				test.registry, test.name, test.expectedImage, got)
		}
	}
}

func Test_LabelsMap(t *testing.T) {
	key1 := "key1"
	key2 := "key2"
	value1 := "value1"
	value2 := "value2"

	t.Setenv("BAD_EXAMPLE", ":invalid")
	valueLocalEnvIncorrect4 := "{{env:BAD_EXAMPLE}}"

	t.Setenv("GOOD_EXAMPLE", "valid")
	valueLocalEnv4 := "{{env:GOOD_EXAMPLE}}"

	tests := []struct {
		name        string
		labels      []Label
		expectErr   bool
		expectedMap map[string]string
	}{
		{
			name: "invalid Labels should return err",
			labels: []Label{
				{
					Value: &value1,
				},
			},
			expectErr: true,
		},
		{
			name: "with valid env var",
			labels: []Label{
				{
					Key:   &key1,
					Value: &valueLocalEnv4,
				},
			},
			expectErr: false,
			expectedMap: map[string]string{
				key1: "valid",
			},
		},
		{
			name: "with invalid env var",
			labels: []Label{
				{
					Key:   &key1,
					Value: &valueLocalEnvIncorrect4,
				},
			},
			expectErr: true,
		},
		{
			name: "empty labels allowed. returns default labels",
			labels: []Label{
				{
					Key: &key1,
				},
			},
			expectErr: false,
			expectedMap: map[string]string{
				key1: "",
			},
		},
		{
			name: "full set of labels",
			labels: []Label{
				{
					Key:   &key1,
					Value: &value1,
				},
				{
					Key:   &key2,
					Value: &value2,
				},
			},
			expectErr: false,
			expectedMap: map[string]string{
				key1: value1,
				key2: value2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Function{
				Name:    "some-function",
				Runtime: "golang",
				Labels:  tt.labels,
			}
			got, err := f.LabelsMap()

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but didn't get an error from LabelsMap")
				}
			} else {
				if err != nil {
					t.Errorf("got unexpected err: %s", err)
				}
			}
			if err == nil {
				defaultLabels := expectedDefaultLabels(f)
				for k, v := range defaultLabels {
					tt.expectedMap[k] = v
				}
				if res := reflect.DeepEqual(got, tt.expectedMap); !res {
					t.Errorf("mismatch in actual and expected labels return. actual: %#v, expected: %#v", got, tt.expectedMap)
				}
			}
		})
	}
}

func expectedDefaultLabels(f Function) map[string]string {
	return map[string]string{
		fnlabels.FunctionKey:                  fnlabels.FunctionValue,
		fnlabels.FunctionNameKey:              f.Name,
		fnlabels.FunctionRuntimeKey:           f.Runtime,
		fnlabels.DeprecatedFunctionKey:        fnlabels.FunctionValue,
		fnlabels.DeprecatedFunctionRuntimeKey: f.Runtime,
	}
}
