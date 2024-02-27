//go:build !integration
// +build !integration

package functions

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v2"
	fnlabels "knative.dev/func/pkg/k8s/labels"

	. "knative.dev/func/pkg/testing"
)

// TestFunction_Validate ensures that we are permissive on what we accept and
// strict on what we emit.  This takes the form of not validating a function
// on instantiation, but rather on write.  A function is expected to be in a
// partial, even invalid state on disk; mostly due to the possibility of manual
// editing of the func.yaml.  Writing, however, should always to write a
// function in a known valid state.
func TestFunction_Validate(t *testing.T) {
	root, cleanup := Mktemp(t)
	t.Cleanup(cleanup)

	var f Function
	var err error

	// Loading a nonexistent (new) function should not fail
	// I.e. it will not run .Validate, or it would error that the function at
	// root has no language or name.
	if f, err = NewFunction(root); err != nil {
		t.Fatal(err)
	}

	// Attempting to write the function will fail as being invalid
	invalidEnv := "*invalid"
	f.Build.BuildEnvs = []Env{{Name: &invalidEnv}}
	if err = f.Write(); err == nil {
		t.Fatalf("expected error writing an incomplete (invalid) function")
	}

	// Write the invalid Function
	//
	// Write this intentionally invalid function to disk.
	// NOTE: this depends on an implementation detail of the package: the yaml
	// serialization of the Function struct to a known filename. This is why this
	// test belongs here in the same package as the implementation rather than in
	// package functions_test which treats the function package as an opaque-box.
	path := filepath.Join(root, FunctionFile)
	bb, err := yaml.Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, bb, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	// Loading the invalid function should not fail, but validation should.
	if f, err = NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if err = f.Validate(); err == nil { // axiom check; not strictly part of this test
		t.Fatal("did not receive an error validating a known-invlaid (incomplete) function")
	}

	// Remove the invalid structures... write should complete without error.
	f.Build.BuildEnvs = []Env{}
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}
	if f, err = NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if err = f.Validate(); err != nil {
		t.Fatal(err)
	}

}

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
		{
			name:   "Full path with port and SHA256 Digest",
			fields: fields{Image: "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@sha256:42", ImageDigest: "sha256:42"},
			want:   "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@sha256:42",
		},
		{
			name:   "Full path with port and SHA256 Digest with empty ImageDigest",
			fields: fields{Image: "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@sha256:42", ImageDigest: ""},
			want:   "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@sha256:42",
		},
	}
	//TODO: gauron99 - this is gonna need to be changed (probably) because:
	// 1: imageDigest now doesnt have a dedicated structure member (resolved?)
	// 2: is still fetched after pushing the Function (which is a temporary fix -- it really should be during build)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Function{
				Build: BuildSpec{
					Image: tt.fields.Image,
				},
			}
			if got := f.ImageNameWithDigest(tt.fields.ImageDigest); got != tt.want {
				t.Errorf("ImageNameWithDigest(tt.fields.ImageDigest) = %v, want %v", got, tt.want)
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
		name          string
		registry      string
		funcName      string
		expectedImage string
		expectError   bool
	}{
		{"short-name", "alice", "myfunc", DefaultRegistry + "/alice/myfunc:latest", false},
		{"short-name-trailing-slash", "alice/", "myfunc", DefaultRegistry + "/alice/myfunc:latest", false},
		{"full-name-quay-io", "quay.io/alice", "myfunc", "quay.io/alice/myfunc:latest", false},
		{"full-name-docker-io", "docker.io/alice", "myfunc", DefaultRegistry + "/alice/myfunc:latest", false},
		{"full-name-with-sub-path", "docker.io/alice/sub", "myfunc", DefaultRegistry + "/alice/sub/myfunc:latest", false},
		{"localhost-direct", "localhost:5000", "myfunc", "localhost:5000/myfunc:latest", false},
		{"full-name-with-sub-sub-path", "us-central1-docker.pkg.dev/my-gcpproject/team/user", "myfunc", "us-central1-docker.pkg.dev/my-gcpproject/team/user/myfunc:latest", false},
		{"missing-func-name", "alice", "", "", true},
		{"missing-registry", "", "myfunc", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f = Function{Registry: test.registry, Name: test.funcName}
			got, err = f.ImageName()
			if test.expectError && err == nil {
				t.Errorf("registry '%v' and name '%v' did not yield the expected error",
					test.registry, test.funcName)
			}
			if !test.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != test.expectedImage {
				t.Errorf("expected registry '%v' name '%v' to yield image '%v', got '%v'",
					test.registry, test.funcName, test.expectedImage, got)
			}
		})
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
				Deploy:  DeploySpec{Labels: tt.labels},
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
