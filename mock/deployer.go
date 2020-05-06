package mock

type Deployer struct {
	DeployInvoked bool
	DeployFn      func(name, image string) (address string, err error)
}

func NewDeployer() *Deployer {
	return &Deployer{
		DeployFn: func(string, string) (string, error) { return "", nil },
	}
}

func (i *Deployer) Deploy(name, image string) (address string, err error) {
	i.DeployInvoked = true
	return i.DeployFn(name, image)
}
