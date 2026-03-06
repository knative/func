//go:build !linux
// +build !linux

package docker

import (
	"github.com/docker/docker/client"
	mobyClient "github.com/moby/moby/client"
)

func newClientWithPodmanService() (dockerClient client.APIClient, dockerHost string, err error) {
	panic("only implemented on Linux")
}

func newMobyClientWithPodmanService() (mobyClient.APIClient, string, error) {
	panic("only implemented on Linux")
}
