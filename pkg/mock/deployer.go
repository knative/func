package mock

import (
	"context"
	"errors"

	fn "knative.dev/func/pkg/functions"
)

// DefaultNamespace for mock deployments
// See deployer implementations for tests which ensure the currently
// active kube namespace is chosen when no explicit namespace is provided.
// This mock emulates a deployer which responds that the function was deployed
// to desired or previously-deployed ns or "default" if not defined.
const DefaultNamespace = "default"

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(context.Context, fn.Function) (fn.DeploymentResult, error)
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(_ context.Context, f fn.Function) (result fn.DeploymentResult, err error) {
			// the minimum necessary logic for a deployer, which should be
			// confirmed by tests in the respective implementations.
			if f.Namespace != "" {
				result.Namespace = f.Namespace // deployed to that requested
			} else if f.Deploy.Namespace != "" {
				result.Namespace = f.Deploy.Namespace // redeploy to current
			} else {
				err = errors.New("namespace required for initial deployment")
			}
			return
		},
	}
}

func (i *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	i.DeployInvoked = true
	return i.DeployFn(ctx, f)
}

// NewDeployerWithResult is a convenience method for creating a mock deployer
// with a deploy function implementation which returns the given result
// and no error.
func NewDeployerWithResult(result fn.DeploymentResult) *Deployer {
	return &Deployer{
		DeployFn: func(context.Context, fn.Function) (fn.DeploymentResult, error) { return result, nil },
	}
}
