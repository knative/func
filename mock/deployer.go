package mock

import (
	"context"

	bosonFunc "github.com/boson-project/func"
)

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(bosonFunc.Function) error
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(bosonFunc.Function) error { return nil },
	}
}

func (i *Deployer) Deploy(ctx context.Context, f bosonFunc.Function) (bosonFunc.DeploymentResult, error) {
	i.DeployInvoked = true
	return bosonFunc.DeploymentResult{}, i.DeployFn(f)
}
