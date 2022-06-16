package docker

import (
	"sync"

	"github.com/docker/docker/client"
)

// Client that panics when used after Close()
type closeGuardingClient struct {
	pimpl  client.CommonAPIClient
	m      sync.RWMutex
	closed bool
}

func (c *closeGuardingClient) Close() error {
	c.m.Lock()
	defer c.m.Unlock()
	c.closed = true
	return c.pimpl.Close()
}
