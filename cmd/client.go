package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/ory/viper"
	"knative.dev/func/pkg/deployers"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/ocproute"

	"knative.dev/func/cmd/prompt"
	"knative.dev/func/pkg/buildpacks"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/creds"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	fnhttp "knative.dev/func/pkg/http"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/operator"
	"knative.dev/func/pkg/pipelines/tekton"
)

// ClientConfig settings for use with NewClient
// These are the minimum settings necessary to create the default client
// instance which has most subsystems initialized.
type ClientConfig struct {
	// Verbose logging.  By default, logging output is kept to the bare minimum.
	// Use this flag to configure verbose logging throughout.
	Verbose bool

	// Allow insecure server connections when using SSL
	InsecureSkipVerify bool

	// K8sClient is the cluster client every cluster-facing component uses.
	// Commands resolve it once and pass it here. If nil, NewClient resolves
	// it from the kubeconfig.
	K8sClient *k8s.Client
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
// the concrete implementations set. Provide additional Options to this constructor
// to override or augment as needed, or override the ClientFactory passed to
// commands entirely to mock for testing. Note the returned cleanup function.
// 'Namespace' is optional.  If not provided (see DefaultNamespace commentary),
// the currently configured is used.
// 'Verbose' indicates the system should write out a higher amount of logging.
func NewClient(cfg ClientConfig, options ...fn.Option) (*fn.Client, func()) {
	var (
		kc = newK8sClient(cfg.K8sClient)
		t  = newTransport(kc, cfg.InsecureSkipVerify)                                // may provide a custom impl which proxies
		c  = newCredentialsProvider(kc, config.Dir(), t, "", cfg.InsecureSkipVerify) // for accessing registries
		d  = newKnativeDeployer(kc, cfg.Verbose)                                     // default deployer (can be overridden via options)
		pp = newTektonPipelinesProvider(kc, c, cfg.Verbose, t)
		o  = []fn.Option{ // standard (shared) options for all commands
			fn.WithVerbose(cfg.Verbose),
			fn.WithTransport(t),
			fn.WithRepositoriesPath(config.RepositoriesPath()),
			fn.WithScaffolder(buildpacks.NewScaffolder(cfg.Verbose)),
			fn.WithBuilder(buildpacks.NewBuilder(buildpacks.WithVerbose(cfg.Verbose))),
			fn.WithRemovers(knative.NewRemover(kc, cfg.Verbose), k8s.NewRemover(kc, cfg.Verbose),
				keda.NewRemover(kc, cfg.Verbose)),
			fn.WithDescribers(
				knative.NewDescriber(kc, cfg.Verbose, knative.WithDescriberTransport(t)),
				k8s.NewDescriber(kc, cfg.Verbose, k8s.WithDescriberTransport(t)),
				keda.NewDescriber(kc, cfg.Verbose, keda.WithDescriberTransport(t)),
			),
			fn.WithListers(knative.NewLister(kc, cfg.Verbose), k8s.NewLister(kc, cfg.Verbose), keda.NewLister(kc, cfg.Verbose)),
			fn.WithDeployer(d),
			fn.WithPipelinesProvider(pp),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(c),
				docker.WithTransport(t),
				docker.WithVerbose(cfg.Verbose),
				docker.WithInsecure(cfg.InsecureSkipVerify))),
			fn.WithSyncer(operator.NewSyncer(operator.WithCredentialsProvider(c))),
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

// newK8sClient returns kc, or a client resolved from the kubeconfig when kc
// is nil. This is the one place a command falls back to the kubeconfig.
func newK8sClient(kc *k8s.Client) *k8s.Client {
	if kc != nil {
		return kc
	}
	return k8s.NewClientFromKubeconfig()
}

// newTransport returns a transport with cluster-flavor-specific variations
// which take advantage of additional features offered by cluster variants.
func newTransport(kc *k8s.Client, insecureSkipVerify bool) fnhttp.RoundTripCloser {
	opts := []fnhttp.Option{
		fnhttp.WithInsecureSkipVerify(insecureSkipVerify),
		fnhttp.WithOpenShiftServiceCA(kc),
	}
	if kc != nil && kc.Loader() != nil {
		opts = append(opts, fnhttp.WithInClusterDialer(k8s.NewLazyInitInClusterDialer(kc)))
	}
	return fnhttp.NewRoundTripper(kc, opts...)
}

// newCredentialsProvider returns a credentials provider which possibly
// has cluster-flavor specific additional credential loaders to take advantage
// of features or configuration nuances of cluster variants.
// If authFilePath is provided (non-empty), it will be used as the primary auth file.
// When insecure is true, credential verification uses plain HTTP instead of HTTPS.
func newCredentialsProvider(kc *k8s.Client, configPath string, t http.RoundTripper, authFilePath string, insecure bool) oci.CredentialsProvider {
	additionalLoaders := append(kc.OpenShiftDockerCredentialLoaders(), k8s.GetGoogleCredentialLoader()...)
	additionalLoaders = append(additionalLoaders, k8s.GetECRCredentialLoader()...)
	additionalLoaders = append(additionalLoaders, k8s.GetACRCredentialLoader()...)

	additionalLoaders = append(additionalLoaders,
		func(registry string) (oci.Credentials, error) {
			uname := viper.GetString("username")
			passw := viper.GetString("password")
			token := viper.GetString("token")
			if (uname != "" && passw != "") || token != "" {
				return oci.Credentials{
					Username: uname,
					Password: passw,
					Token:    token,
				}, nil
			}
			return oci.Credentials{}, creds.ErrCredentialsNotFound
		},
	)

	options := []creds.Opt{
		creds.WithPromptForCredentials(prompt.NewPromptForCredentials(os.Stdin, os.Stdout, os.Stderr)),
		creds.WithPromptForCredentialStore(prompt.NewPromptForCredentialStore()),
		creds.WithTransport(t),
		creds.WithInsecure(insecure),
		creds.WithAdditionalCredentialLoaders(additionalLoaders...),
	}

	// If a custom auth file path is provided, use it
	if authFilePath != "" {
		options = append(options, creds.WithAuthFilePath(authFilePath))
	}

	// Other cluster variants can be supported here
	return creds.NewCredentialsProvider(configPath, options...)
}

func newTektonPipelinesProvider(kc *k8s.Client, creds oci.CredentialsProvider, verbose bool, transport http.RoundTripper) *tekton.PipelinesProvider {
	options := []tekton.Opt{
		tekton.WithCredentialsProvider(creds),
		tekton.WithVerbose(verbose),
		tekton.WithPipelineDecorator(deployDecorator{kc}),
		tekton.WithTransport(transport),
		tekton.WithK8sClient(kc),
	}

	return tekton.NewPipelinesProvider(options...)
}

func newKnativeDeployer(kc *k8s.Client, verbose bool) fn.Deployer {
	return knative.NewDeployer(kc,
		knative.WithDeployerVerbose(verbose),
		knative.WithDeployerDecorator(deployDecorator{kc}),
	)
}

// newK8sDeployer builds the raw deployer.
//
// The Exposer is attached unconditionally, not only when the deploy asks for a
// Route. The record saying whether teardown is owed lives on the cluster, so
// wiring time cannot know.
func newK8sDeployer(kc *k8s.Client, verbose bool) fn.Deployer {
	return k8s.NewDeployer(kc,
		k8s.WithDeployerVerbose(verbose),
		k8s.WithDeployerDecorator(deployDecorator{kc}),
		k8s.WithExposer(ocproute.New(deployers.Kubernetes)),
	)
}

// newKedaDeployer builds the keda deployer. The Exposer is keda's own, never
// the embedded raw deployer's, so Routes point at the interceptor rather than
// bypassing it. Attached unconditionally for the reason in newK8sDeployer,
// which bites harder here: keda's Route has no owner reference, so a Route
// nothing goes looking for is a Route nothing ever removes.
func newKedaDeployer(kc *k8s.Client, verbose bool) fn.Deployer {
	return keda.NewDeployer(kc,
		keda.WithDeployerVerbose(verbose),
		keda.WithDeployerDecorator(deployDecorator{kc}),
		keda.WithExposer(ocproute.New(deployers.Keda)),
	)
}

// deployDecorator adds OpenShift metadata when the target cluster is
// OpenShift.
type deployDecorator struct {
	kc *k8s.Client
}

func (d deployDecorator) UpdateAnnotations(function fn.Function, annotations map[string]string) map[string]string {
	if ok, _ := d.kc.IsOpenShift(); ok {
		return k8s.OpenshiftMetadataDecorator{}.UpdateAnnotations(function, annotations)
	}
	return annotations
}

func (d deployDecorator) UpdateLabels(function fn.Function, labels map[string]string) map[string]string {
	if ok, _ := d.kc.IsOpenShift(); ok {
		return k8s.OpenshiftMetadataDecorator{}.UpdateLabels(function, labels)
	}
	return labels
}
