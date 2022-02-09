package cmd

import (
	"net/http"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/docker/creds"
	"knative.dev/kn-plugin-func/knative"
	"knative.dev/kn-plugin-func/pipelines/tekton"
	"knative.dev/kn-plugin-func/progress"
)

type ClientOptions struct {
	Namespace    string
	Registry     string
	Repository   string
	Repositories string
	Verbose      bool
}

type ClientFactory func(opts ClientOptions) *fn.Client

func NewClientFactory(transport http.RoundTripper) ClientFactory {
	return func(clientOptions ClientOptions) *fn.Client {
		builder := buildpacks.NewBuilder()
		builder.Verbose = clientOptions.Verbose

		progressListener := progress.New()
		progressListener.Verbose = clientOptions.Verbose

		credentialsProvider := creds.NewCredentialsProvider(
			creds.WithPromptForCredentials(newPromptForCredentials()),
			creds.WithPromptForCredentialStore(newPromptForCredentialStore()),
			creds.WithTransport(transport))

		pusher := docker.NewPusher(
			docker.WithCredentialsProvider(credentialsProvider),
			docker.WithProgressListener(progressListener),
			docker.WithTransport(transport))
		pusher.Verbose = clientOptions.Verbose

		deployer := knative.NewDeployer(clientOptions.Namespace)
		deployer.Verbose = clientOptions.Verbose

		pipelinesProvider := tekton.NewPipelinesProvider(
			tekton.WithNamespace(clientOptions.Namespace),
			tekton.WithProgressListener(progressListener),
			tekton.WithCredentialsProvider(credentialsProvider))
		pipelinesProvider.Verbose = clientOptions.Verbose

		remover := knative.NewRemover(clientOptions.Namespace)
		remover.Verbose = clientOptions.Verbose

		describer := knative.NewDescriber(clientOptions.Namespace)
		describer.Verbose = clientOptions.Verbose

		lister := knative.NewLister(clientOptions.Namespace)
		lister.Verbose = clientOptions.Verbose

		runner := docker.NewRunner()
		runner.Verbose = clientOptions.Verbose

		opts := []fn.Option{
			fn.WithRepository(clientOptions.Repository), // URI of repository override
			fn.WithRegistry(clientOptions.Registry),
			fn.WithVerbose(clientOptions.Verbose),
			fn.WithTransport(transport),
			fn.WithProgressListener(progressListener),
			fn.WithBuilder(builder),
			fn.WithPipelinesProvider(pipelinesProvider),
			fn.WithRemover(remover),
			fn.WithDescriber(describer),
			fn.WithLister(lister),
			fn.WithRunner(runner),
			fn.WithDeployer(deployer),
			fn.WithPusher(pusher),
		}

		if clientOptions.Repositories != "" {
			opts = append(opts, fn.WithRepositories(clientOptions.Repositories)) // path to repositories in disk
		}

		return fn.New(opts...)
	}
}
