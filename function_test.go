package faas

import "testing"

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
			name: "Full path with port",
			fields: fields{Image: "image-registry.openshift-image-registry.svc.cluster.local:5000/default/bar", ImageDigest: "42"},
			want:   "image-registry.openshift-image-registry.svc.cluster.local:5000/default/bar@42",
		},
		{
			name: "Path with namespace",
			fields: fields{Image: "johndoe/bar", ImageDigest: "42"},
			want:   "johndoe/bar@42",
		},
		{
			name: "Just image name",
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
