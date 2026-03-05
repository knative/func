package docker

import (
	"sync"

	mobyClient "github.com/moby/moby/client"
)

//go:generate go run ../../hack/cmd/gen-close-guard/

// Client that panics when used after Close()
type closeGuardingClient struct {
	pimpl   *mobyClient.Client
	m       sync.RWMutex
	closed  bool
	cleanUp func() // optional cleanup on Close
}

func (c *closeGuardingClient) Close() error {
	c.m.Lock()
	defer c.m.Unlock()
	c.closed = true
	err := c.pimpl.Close()
	if c.cleanUp != nil {
		c.cleanUp()
	}
	return err
}
