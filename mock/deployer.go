package mock

import function "github.com/boson-project/func"

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(function.Function) error
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(function.Function) error { return nil },
	}
}

func (i *Deployer) Deploy(f function.Function) error {
	i.DeployInvoked = true
	return i.DeployFn(f)
}
