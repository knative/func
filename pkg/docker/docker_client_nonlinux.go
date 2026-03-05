//go:build !linux

package docker

import (
	mobyClient "github.com/moby/moby/client"
)

func newClientWithPodmanService() (*mobyClient.Client, string, error) {
	panic("only implemented on Linux")
}
