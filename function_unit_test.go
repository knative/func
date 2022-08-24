//go:build !integration
// +build !integration

package function

import (
	"testing"
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
