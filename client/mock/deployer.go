package mock

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(name, path string) (address string, err error)
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(string, string) (string, error) { return "", nil },
	}
}

func (i *Deployer) Deploy(name, path string) (address string, err error) {
	i.DeployInvoked = true
	return i.DeployFn(name, path)
}
