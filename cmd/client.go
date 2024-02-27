package cmd

import (
	"fmt"
	"net/http"
	"os"

	"knative.dev/func/cmd/prompt"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/docker"
	"knative.dev/func/pkg/docker/creds"
	fn "knative.dev/func/pkg/functions"
	fnhttp "knative.dev/func/pkg/http"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/pipelines/tekton"
)

// ClientConfig settings for use with NewClient
// These are the minimum settings necessary to create the default client
// instance which has most subsystems initialized.
type ClientConfig struct {
	// Namespace in the remote cluster to use for any client commands which
	// touch the remote.  Optional.  Empty namespace indicates the namespace
	// currently configured in the client's connection should be used.
	Namespace string

	// Verbose logging.  By default, logging output is kept to the bare minimum.
	// Use this flag to configure verbose logging throughout.
	Verbose bool

	// Allow insecure server connections when using SSL
	InsecureSkipVerify bool
}

// ClientFactory defines a constructor which assists in the creation of a Client
// for use by commands.
// See the NewClient constructor which is the fully populated ClientFactory used
// by commands by default.
// See NewClientFactory which constructs a minimal ClientFactory for use
// during testing.
type ClientFactory func(ClientConfig, ...fn.Option) (*fn.Client, func())

// NewTestClient returns a client factory which will ignore options used,
// instead using those provided when creating the factory.  This allows
// for tests to create an entirely default client but with N mocks.
func NewTestClient(options ...fn.Option) ClientFactory {
	return func(_ ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		return fn.New(options...), func() {}
	}
}

// NewClient constructs an fn.Client with the majority of
// the concrete implementations set.  Provide additional Options to this constructor
// to override or augment as needed, or override the ClientFactory passed to
// commands entirely to mock for testing. Note the returned cleanup function.
// 'Namespace' is optional.  If not provided (see DefaultNamespace commentary),
// the currently configured is used.
// 'Verbose' indicates the system should write out a higher amount of logging.
func NewClient(cfg ClientConfig, options ...fn.Option) (*fn.Client, func()) {
	var (
		t  = newTransport(cfg.InsecureSkipVerify)    // may provide a custom impl which proxies
		c  = newCredentialsProvider(config.Dir(), t) // for accessing registries
		d  = newKnativeDeployer(cfg.Namespace, cfg.Verbose)
		pp = newTektonPipelinesProvider(cfg.Namespace, c, cfg.Verbose)
		o  = []fn.Option{ // standard (shared) options for all commands
			fn.WithVerbose(cfg.Verbose),
			fn.WithTransport(t),
			fn.WithRepositoriesPath(config.RepositoriesPath()),
			fn.WithBuilder(buildpacks.NewBuilder(buildpacks.WithVerbose(cfg.Verbose))),
			fn.WithRemover(knative.NewRemover(cfg.Verbose)),
			fn.WithDescriber(knative.NewDescriber(cfg.Namespace, cfg.Verbose)),
			fn.WithLister(knative.NewLister(cfg.Namespace, cfg.Verbose)),
			fn.WithDeployer(d),
			fn.WithPipelinesProvider(pp),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(c),
				docker.WithTransport(t),
				docker.WithVerbose(cfg.Verbose))),
		}
	)

	// Client is constructed with standard options plus any additional options
	// which either augment or override the defaults.
	client := fn.New(append(o, options...)...)

	// A deferrable cleanup function which is used to perform any cleanup, such
	// as closing the transport
	cleanup := func() {
		if err := t.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error closing http transport. %v", err)
		}
	}

	return client, cleanup
}

// newTransport returns a transport with cluster-flavor-specific variations
// which take advantage of additional features offered by cluster variants.
func newTransport(insecureSkipVerify bool) fnhttp.RoundTripCloser {
	return fnhttp.NewRoundTripper(fnhttp.WithInsecureSkipVerify(insecureSkipVerify), fnhttp.WithOpenShiftServiceCA())
}

// newCredentialsProvider returns a credentials provider which possibly
// has cluster-flavor specific additional credential loaders to take advantage
// of features or configuration nuances of cluster variants.
func newCredentialsProvider(configPath string, t http.RoundTripper) docker.CredentialsProvider {
	options := []creds.Opt{
		creds.WithPromptForCredentials(prompt.NewPromptForCredentials(os.Stdin, os.Stdout, os.Stderr)),
		creds.WithPromptForCredentialStore(prompt.NewPromptForCredentialStore()),
		creds.WithTransport(t),
		creds.WithAdditionalCredentialLoaders(k8s.GetOpenShiftDockerCredentialLoaders()...),
	}

	// Other cluster variants can be supported here
	return creds.NewCredentialsProvider(configPath, options...)
}

func newTektonPipelinesProvider(namespace string, creds docker.CredentialsProvider, verbose bool) *tekton.PipelinesProvider {
	options := []tekton.Opt{
		tekton.WithNamespace(namespace),
		tekton.WithCredentialsProvider(creds),
		tekton.WithVerbose(verbose),
		tekton.WithPipelineDecorator(deployDecorator{}),
	}

	return tekton.NewPipelinesProvider(options...)
}

func newKnativeDeployer(namespace string, verbose bool) fn.Deployer {
	options := []knative.DeployerOpt{
		knative.WithDeployerNamespace(namespace),
		knative.WithDeployerVerbose(verbose),
		knative.WithDeployerDecorator(deployDecorator{}),
	}

	return knative.NewDeployer(options...)
}

type deployDecorator struct {
	oshDec k8s.OpenshiftMetadataDecorator
}

func (d deployDecorator) UpdateAnnotations(function fn.Function, annotations map[string]string) map[string]string {
	if k8s.IsOpenShift() {
		return d.oshDec.UpdateAnnotations(function, annotations)
	}
	return annotations
}

func (d deployDecorator) UpdateLabels(function fn.Function, labels map[string]string) map[string]string {
	if k8s.IsOpenShift() {
		return d.oshDec.UpdateLabels(function, labels)
	}
	return labels
}
