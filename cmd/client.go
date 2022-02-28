package cmd

import (
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

// NewClient creates an fn.Client with the majority of the concrete
// implementations defaulted.  Provide additional Options to this constructor
// to override or augment as needed.  Returned is also a cleanup function
// which is the responsibility of the caller to invoke to free potentially
// dedicated resources. To free these resources (after instances of
// function.Client are no longer in use) the caller of this constructor should
// invoke the cleanup routine.
// 'Namespace' is optional.  If not provided, the
// currently configured namespace will be used.
// 'Verbose' indicates the system should write out a higher amount of information
// about its execution.
// Usage:
//   client, cleanup:= NewClient("",false)
//   defer cleanup()
func NewClient(namespace string, verbose bool, options ...fn.Option) (*fn.Client, func()) {
	var (
		p = progress.New(verbose) // updates the CLI
		t = newTransport()
		c = newCredentialsProvider(t)
	)
	client := fn.New(
		fn.WithVerbose(verbose),
		fn.WithProgressListener(p),
		fn.WithTransport(t),
		fn.WithBuilder(buildpacks.NewBuilder(verbose)),
		fn.WithRemover(knative.NewRemover(namespace, verbose)),
		fn.WithDescriber(knative.NewDescriber(namespace, verbose)),
		fn.WithLister(knative.NewLister(namespace, verbose)),
		fn.WithRunner(docker.NewRunner(verbose)),
		fn.WithDeployer(knative.NewDeployer(namespace, verbose)),
		fn.WithPipelinesProvider(tekton.NewPipelinesProvider(
			tekton.WithNamespace(namespace),
			tekton.WithProgressListener(p),
			tekton.WithCredentialsProvider(c),
			tekton.WithVerbose(verbose))),
		fn.WithPusher(docker.NewPusher(
			docker.WithCredentialsProvider(c),
			docker.WithProgressListener(p),
			docker.WithTransport(t),
			docker.WithVerbose(verbose))),
		options..., // override or set additional options
	)
	cleanup := func() {
		t.Close() // the custom transport needs to be closed.
	}
	return client, cleanup
}

// newTransport returns a transport with cluster-flavor-specific variations
// which take advantage of additional features offered by cluster variants.
func newTransport(verbose bool) fnhttp.RoundTripCloser {
	if openshift.IsOpenShift() {
		return fnhttp.NewRoundTripper(openshift.WithOpenShiftServiceCA())
	}
	// Other cluster variants forthcoming...

	return fnhttp.NewRoundTripper() // Default transport
}

// newCredentialsProvider returns a credentials provider which possibly
// has cluster-flavor specific additional credential loaders to take advantage
// of features or configuration nuances of cluster variants.
func newCredentailsProvider(t docker.CredentialsProvider) docker.CredentialsProvider {
	options := []creds.Option{
		creds.WithPromptForCredentials(newPromptForCredentials()),
		creds.WithPromptForCredentialStore(newPromptForCredentialStore()),
		creds.WithTransport(t),
	}
	// The OpenShift variant has additional ways to load credentials
	if openshift.IsOpenShift() {
		options = append(options,
			creds.WithAdditionalCredentialLoaders(openshift.GetDockerCredentialLoaders()))
	}
	// Other cluster variants can be supported here
	return creds.NewCredentialsProvider(options...)
}

func GetDefaultRegistry() string {
	switch {
	case openshift.IsOpenShift():
		return openshift.GetDefaultRegistry()
	default:
		return ""
	}
}
