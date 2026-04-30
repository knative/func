package docker

import (
	"sync"
)

// closeGuardingClient wraps a DockerClient and panics if any method
// is called after Close().
type closeGuardingClient struct {
	pimpl  DockerClient
	m      sync.RWMutex
	closed bool
}

func (c *closeGuardingClient) Close() error {
	c.m.Lock()
	defer c.m.Unlock()
	c.closed = true
	return c.pimpl.Close()
}
