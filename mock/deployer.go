package mock

import (
	"context"

	fn "knative.dev/kn-plugin-func"
)

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(fn.Function) error
	DeployResult  *fn.DeploymentResult
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(fn.Function) error { return nil },
	}
}

func NewDeployerWithResult(result *fn.DeploymentResult) *Deployer {
	return &Deployer{
		DeployFn:     func(fn.Function) error { return nil },
		DeployResult: result,
	}
}

func (i *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	i.DeployInvoked = true
	if i.DeployResult != nil {
		return *i.DeployResult, i.DeployFn(f)
	}
	return fn.DeploymentResult{}, i.DeployFn(f)
}
