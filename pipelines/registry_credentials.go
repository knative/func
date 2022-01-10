package pipelines

type ContainerRegistryCredentialsCallback func() (ContainerRegistryCredentials, error)

type ContainerRegistryCredentials struct {
	Username string
	Password string
	Server   string
}
