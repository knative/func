package pipelines

// PacMetadata holds data needed for Pipelines as Code generation,
// these data are transient and confidential (credentials)
// so we most liketly don't want to store them in function config.
type PacMetadata struct {
	RegistryUsername string
	RegistryPassword string
	RegistryServer   string

	GitProvider string

	PersonalAccessToken string
	WebhookSecret       string

	ConfigureLocalResources   bool
	ConfigureClusterResources bool
	ConfigureRemoteResources  bool
}
