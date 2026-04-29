//go:build !linux

package docker

import "github.com/moby/moby/client"

func newClientWithPodmanService() (dockerClient client.APIClient, dockerHost string, err error) {
	panic("only implemented on Linux")
}
