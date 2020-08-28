package mock

import "github.com/boson-project/faas"

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(faas.Function) error
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(faas.Function) error { return nil },
	}
}

func (i *Deployer) Deploy(f faas.Function) error {
	i.DeployInvoked = true
	return i.DeployFn(f)
}
