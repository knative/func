package k8s

import "testing"

func TestIsOpenShiftInternalRegistry(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		want     bool
	}{
		{"internal with port and namespace", "image-registry.openshift-image-registry.svc:5000/mynamespace", true},
		{"internal without port", "image-registry.openshift-image-registry.svc/mynamespace", true},
		{"internal host only", "image-registry.openshift-image-registry.svc", true},
		{"docker.io", "docker.io/user", false},
		{"ghcr.io", "ghcr.io/user", false},
		{"quay.io", "quay.io/user", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOpenShiftInternalRegistry(tt.registry); got != tt.want {
				t.Errorf("expected IsOpenShiftInternalRegistry(%q) = %v, got %v", tt.registry, got, tt.want)
			}
		})
	}
}
