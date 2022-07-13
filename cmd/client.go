package cmd

import (
	"fmt"
	"net/http"
	"os"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/docker/creds"
	fnhttp "knative.dev/kn-plugin-func/http"
	"knative.dev/kn-plugin-func/knative"
	"knative.dev/kn-plugin-func/openshift"
	"knative.dev/kn-plugin-func/pipelines/tekton"
	"knative.dev/kn-plugin-func/progress"
)

// ClientConfig settings for use with NewClient
// These are the minimum settings necessary to create the default client
// instance which has most subsystems initialized.
type ClientConfig struct {
	// Namespace in the remote cluster to use for any client commands which
	// touch the remote.  Optional.  Empty namespace indicates the namespace
	// currently configured in the client's connection should be used.
	Namespace string

	// Verbose logging.  By default logging output is kept to the bare minimum.
	// Use this flag to configure verbose logging throughout.
	Verbose bool
}

// ClientFactory defines a constructor which assists in the creation of a Client
// for use by commands.
// See the NewClient constructor which is the fully populated ClientFactory used
// by commands by default.
// See NewClientFactory which constructs a minimal CientFactory for use
// during testing.
type ClientFactory func(ClientConfig, ...fn.Option) (*fn.Client, func())

// NewClientFactory enables simple instantiation of an fn.Client, such as
// for mocking during tests or for minimal api usage.
// Given is a minimal Client constructor, Returned is full ClientFactory
// with the aspects of normal (full) Client construction (namespace, verbosity
// level, additional options and the returned cleanup function) ignored.
func NewClientFactory(n func() *fn.Client) ClientFactory {
	return func(_ ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		return n(), func() {}
	}
}

// NewClient constructs an fn.Client with the majority of
// the concrete implementations set.  Provide additional Options to this constructor
// to override or augment as needed, or override the ClientFactory passed to
// commands entirely to mock for testing. Note the reutrned cleanup function.
// 'Namespace' is optional.  If not provided (see DefaultNamespace commentary),
// the currently configured is used.
// 'Verbose' indicates the system should write out a higher amount of logging.
// Example:
//   client, done := NewClient("",false)
//   defer done()
func NewClient(cfg ClientConfig, options ...fn.Option) (*fn.Client, func()) {
	var (
		p  = progress.New(cfg.Verbose) // updates the CLI
		t  = newTransport()            // may provide a custom impl which proxies
		c  = newCredentialsProvider(t) // for accessing registries
		d  = newKnativeDeployer(cfg.Namespace, cfg.Verbose)
		pp = newTektonPipelinesProvider(cfg.Namespace, p, c, cfg.Verbose)
		o  = []fn.Option{ // standard (shared) options for all commands
			fn.WithVerbose(cfg.Verbose),
			fn.WithProgressListener(p),
			fn.WithTransport(t),
			fn.WithBuilder(buildpacks.NewBuilder(buildpacks.WithVerbose(cfg.Verbose))),
			fn.WithRemover(knative.NewRemover(cfg.Namespace, cfg.Verbose)),
			fn.WithDescriber(knative.NewDescriber(cfg.Namespace, cfg.Verbose)),
			fn.WithLister(knative.NewLister(cfg.Namespace, cfg.Verbose)),
			fn.WithRunner(docker.NewRunner(cfg.Verbose)),
			fn.WithDeployer(d),
			fn.WithPipelinesProvider(pp),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(c),
				docker.WithProgressListener(p),
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
func newTransport() fnhttp.RoundTripCloser {
	if openshift.IsOpenShift() {
		return fnhttp.NewRoundTripper(openshift.WithOpenShiftServiceCA())
	}

	// Other cluster variants ...

	return fnhttp.NewRoundTripper() // Default (vanilla k8s)
}

// newCredentialsProvider returns a credentials provider which possibly
// has cluster-flavor specific additional credential loaders to take advantage
// of features or configuration nuances of cluster variants.
func newCredentialsProvider(t http.RoundTripper) docker.CredentialsProvider {
	options := []creds.Opt{
		creds.WithPromptForCredentials(newPromptForCredentials(os.Stdin, os.Stdout, os.Stderr)),
		creds.WithPromptForCredentialStore(newPromptForCredentialStore()),
		creds.WithTransport(t),
	}
	// The OpenShift variant has additional ways to load credentials
	if openshift.IsOpenShift() {
		options = append(options,
			creds.WithAdditionalCredentialLoaders(openshift.GetDockerCredentialLoaders()...))
	}
	// Other cluster variants can be supported here
	return creds.NewCredentialsProvider(options...)
}

func newTektonPipelinesProvider(namespace string, progress *progress.Bar, creds docker.CredentialsProvider, verbose bool) *tekton.PipelinesProvider {
	options := []tekton.Opt{
		tekton.WithNamespace(namespace),
		tekton.WithProgressListener(progress),
		tekton.WithCredentialsProvider(creds),
		tekton.WithVerbose(verbose),
	}

	if openshift.IsOpenShift() {
		options = append(options, tekton.WithPipelineDecorator(openshift.OpenshiftMetadataDecorator{}))
	}

	return tekton.NewPipelinesProvider(options...)

}

func newKnativeDeployer(namespace string, verbose bool) fn.Deployer {
	options := []knative.DeployerOpt{
		knative.WithDeployerNamespace(namespace),
		knative.WithDeployerVerbose(verbose),
	}

	if openshift.IsOpenShift() {
		options = append(options, knative.WithDeployerDecorator(openshift.OpenshiftMetadataDecorator{}))
	}

	return knative.NewDeployer(options...)
}

func GetDefaultRegistry() string {
	switch {
	case openshift.IsOpenShift():
		return openshift.GetDefaultRegistry()
	default:
		return ""
	}
}
