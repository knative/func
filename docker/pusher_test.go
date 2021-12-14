//go:build !integration
// +build !integration

package docker_test

import (
	"testing"

	"knative.dev/kn-plugin-func/docker"
)

func TestParseDigest(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "basic test",
			arg:  "latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613",
			want: "sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := docker.ParseDigest(tt.arg); got != tt.want {
				t.Errorf("ParseDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRegistry(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "default registry",
			arg:  "docker.io/mysamplefunc:latest",
			want: "docker.io",
		},
		{
			name: "long-form nested url",
			arg:  "myregistry.io/myorg/myuser/myfunctions/mysamplefunc:latest",
			want: "myregistry.io",
		},
		{
			name: "invalid url",
			arg:  "myregistry.io-mysamplefunc:latest",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := docker.GetRegistry(tt.arg); got != tt.want {
				t.Errorf("GetRegistry() = %v, want %v", got, tt.want)
			}
		})
	}
}
