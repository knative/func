//go:build !linux
// +build !linux

package docker

import "github.com/docker/docker/client"

func newClientWithPodmanService() (dockerClient client.CommonAPIClient, dockerHost string, err error) {
	panic("only implemented on Linux")
}
