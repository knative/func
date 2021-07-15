package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(fn.Function) error
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(fn.Function) error { return nil },
	}
}

func (i *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	i.DeployInvoked = true
	return fn.DeploymentResult{}, i.DeployFn(f)
}
