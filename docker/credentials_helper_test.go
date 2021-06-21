package docker

import "testing"

func Test_to2ndLevelDomain(t *testing.T) {
	tests := []struct {
		name string
		rawurl string
		want string
	}{
		{"2nd level", "quay.io", "quay.io"},
		{"3nd level", "sub.quay.io", "quay.io"},
		{"localhost", "localhost", "localhost"},
		{"2nd level with protocol", "https://docker.io", "docker.io"},
		{"2nd level with path", "docker.io/v1/", "docker.io"},
		{"2nd level with port", "docker.io:80", "docker.io"},
		{"2nd level with protocol and path", "https://docker.io/v1/", "docker.io"},
		{"3rd level with protocol and path", "https://index.docker.io/v1/", "docker.io"},
		{"3rd level with protocol and path and port", "https://index.docker.io:80/v1/", "docker.io"},
		{"localhost with protocol and path and port", "http://localhost:8080/v1/", "localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := to2ndLevelDomain(tt.rawurl); got != tt.want {
				t.Errorf("to2ndLevelDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

