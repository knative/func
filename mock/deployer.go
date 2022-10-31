package mock

import (
	"context"

	fn "knative.dev/func"
)

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(context.Context, fn.Function) (fn.DeploymentResult, error)
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(context.Context, fn.Function) (fn.DeploymentResult, error) { return fn.DeploymentResult{}, nil },
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
