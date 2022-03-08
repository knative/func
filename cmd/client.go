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

// DefaultNamespace is empty, indicating the namespace currently configured
// in the client's connection should be used.
const DefaultNamespace = ""

// NewClient creates an fn.Client with the majority of the concrete
// implementations defaulted.  Provide additional Options to this constructor
// to override or augment as needed. Note the reutrned cleanup function.
// 'Namespace' is optional.  If not provided (see DefaultNamespace), the
// currently configured is used.
// 'Verbose' indicates the system should write out a higher amount of logging.
// Example:
//   client, done := NewClient("",false)
//   defer done()
func NewClient(namespace string, verbose bool, options ...fn.Option) (*fn.Client, func()) {
	var (
		p = progress.New(verbose)     // updates the CLI
		t = newTransport()            // may provide a custom impl which proxies
		c = newCredentialsProvider(t) // for accessing registries
		o = []fn.Option{              // standard (shared) options for all commands
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
		creds.WithPromptForCredentials(newPromptForCredentials()),
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

func GetDefaultRegistry() string {
	switch {
	case openshift.IsOpenShift():
		return openshift.GetDefaultRegistry()
	default:
		return ""
	}
}
