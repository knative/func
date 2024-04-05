package functions

import (
	"context"
	"errors"
	"fmt"
)

const (
	EnvironmentLocal  = "local"
	EnvironmentRemote = "remote"
)

// InstanceRefs manager
//
// InstanceRefs are point-in-time snapshots of a function's runtime state in
// a given environment.  By default 'local' and 'remote' environmnts are
// available when a function is run locally and deployed (respectively).
type InstanceRefs struct {
	client *Client
}

// newInstances creates a new manager of instances.
func newInstances(client *Client) *InstanceRefs {
	return &InstanceRefs{client: client}
}

// Get the instance data for a function in the named environment.
// For convenient access to the default 'local' and 'remote' environment
// see the Local and Remote methods, respectively.
// Instance returned is populated with a point-in-time snapshot of the
// function state in the named environment.
func (s *InstanceRefs) Get(ctx context.Context, f Function, environment string) (Instance, error) {
	switch environment {
	case EnvironmentLocal:
		return s.Local(ctx, f)
	case EnvironmentRemote:
		return s.Remote(ctx, f.Name, f.Deploy.Namespace)
	default:
		// Future versions will support additional ad-hoc named environments, such
		// as for testing. Local and remote remaining the base cases.
		return Instance{}, ErrEnvironmentNotFound
	}
}

// Local instance details for the function
// If the function is not running locally the error returned is ErrNotRunning
func (s *InstanceRefs) Local(ctx context.Context, f Function) (Instance, error) {
	var i Instance
	// To create a local instance the function must have a root path defined
	// which contains an initialized function and be running.
	if f.Root == "" {
		return i, ErrRootRequired
	}
	if !f.Initialized() {
		return i, ErrNotInitialized{f.Root}
	}
	ports := jobPorts(f)
	if len(ports) == 0 {
		return i, ErrNotRunning
	}

	route := fmt.Sprintf("http://localhost:%s/", ports[0])

	return Instance{
		Route:  route,
		Routes: []string{route},
		Name:   f.Name,
	}, nil
}

// Remote instance details for the function
//
// Since this is specific to the implicitly available 'remote' environment, the
// request can be completed with either a name or the local source. Therefore
// either name or root path can be passed.  If name is not passed, the function
// at root is loaded and its name used for describing the remote instance.
// Name takes precedence.
func (s *InstanceRefs) Remote(ctx context.Context, name, namespace string) (i Instance, err error) {
	if name == "" {
		return i, errors.New("fetching remote instances requires function name")
	}
	if namespace == "" {
		return i, errors.New("fetching remote instances requires namespace")
	}

	return s.client.describer.Describe(ctx, name, namespace)
}
