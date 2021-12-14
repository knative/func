//go:build !integration
// +build !integration

package function

import (
	"testing"

	. "knative.dev/kn-plugin-func/testing"
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

func Test_DerivedImage(t *testing.T) {
	tests := []struct {
		name     string
		fnName   string
		image    string
		registry string
		want     string
	}{
		{
			name:     "No change",
			fnName:   "testDerivedImage",
			image:    "docker.io/foo/testDerivedImage:latest",
			registry: "docker.io/foo",
			want:     "docker.io/foo/testDerivedImage:latest",
		},
		{
			name:     "Same registry without docker.io/, original with docker.io/",
			fnName:   "testDerivedImage0",
			image:    "docker.io/foo/testDerivedImage0:latest",
			registry: "foo",
			want:     "docker.io/foo/testDerivedImage0:latest",
		},
		{
			name:     "Same registry, original without docker.io/",
			fnName:   "testDerivedImage1",
			image:    "foo/testDerivedImage1:latest",
			registry: "foo",
			want:     "docker.io/foo/testDerivedImage1:latest",
		},
		{
			name:     "Different registry without docker.io/, original without docker.io/",
			fnName:   "testDerivedImage2",
			image:    "foo/testDerivedImage2:latest",
			registry: "bar",
			want:     "docker.io/bar/testDerivedImage2:latest",
		},
		{
			name:     "Different registry with docker.io/, original without docker.io/",
			fnName:   "testDerivedImage3",
			image:    "foo/testDerivedImage3:latest",
			registry: "docker.io/bar",
			want:     "docker.io/bar/testDerivedImage3:latest",
		},
		{
			name:     "Different registry with docker.io/, original with docker.io/",
			fnName:   "testDerivedImage4",
			image:    "docker.io/foo/testDerivedImage4:latest",
			registry: "docker.io/bar",
			want:     "docker.io/bar/testDerivedImage4:latest",
		},
		{
			name:     "Different registry with quay.io/, original without docker.io/",
			fnName:   "testDerivedImage5",
			image:    "foo/testDerivedImage5:latest",
			registry: "quay.io/foo",
			want:     "quay.io/foo/testDerivedImage5:latest",
		},
		{
			name:     "Different registry with quay.io/, original with docker.io/",
			fnName:   "testDerivedImage6",
			image:    "docker.io/foo/testDerivedImage6:latest",
			registry: "quay.io/foo",
			want:     "quay.io/foo/testDerivedImage6:latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			root := "testdata/" + tt.fnName
			defer Using(t, root)()

			// write out the function
			client := New()
			err := client.Create(Function{Runtime: "go", Name: tt.fnName, Root: root})
			if err != nil {
				t.Fatal(err)
			}

			got, err := DerivedImage(root, tt.registry)
			if err != nil {
				t.Errorf("DerivedImage() for image %v and registry %v; got error %v", tt.image, tt.registry, err)
			}
			if got != tt.want {
				t.Errorf("DerivedImage() for image %v and registry %v; got %v, want %v", tt.image, tt.registry, got, tt.want)
			}
		})
	}
}
