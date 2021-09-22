package docker

import "testing"

func Test_registryEquals(t *testing.T) {
	tests := []struct {
		name string
		urlA string
		urlB string
		want bool
	}{
		{"no port matching host", "quay.io", "quay.io", true},
		{"non-matching host different sub-domain", "sub.quay.io", "quay.io", false},
		{"non-matching host different sub-domain", "sub.quay.io", "sub3.quay.io", false},
		{"localhost", "localhost", "localhost", true},
		{"localhost with standard ports", "localhost:80", "localhost:443", true},
		{"localhost with matching port", "https://localhost:1234", "http://localhost:1234", true},
		{"localhost with match by default port", "http://localhost", "localhost:80", true},
		{"localhost with match by default port", "https://localhost", "localhost:443", true},
		{"localhost with mismatch by default port", "https://localhost", "http://localhost", true},
		{"docker.io matching host", "https://docker.io", "docker.io", true},
		{"docker.io matching host", "http://docker.io", "docker.io", true},
		{"docker.io with path", "docker.io/v1/", "docker.io", true},
		{"docker.io with protocol and path", "https://docker.io/v1/", "docker.io", true},
		{"docker.io with subdomain index.", "https://index.docker.io/v1/", "docker.io", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registryEquals(tt.urlA, tt.urlB); got != tt.want {
				t.Errorf("to2ndLevelDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}
