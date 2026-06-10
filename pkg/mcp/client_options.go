package mcp

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/buildpacks"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/creds"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	fnhttp "knative.dev/func/pkg/http"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/s2i"
)

// builderClientOptions returns client options for the given builder name.
func builderClientOptions(builder string, cfg ClientConfig, withTimestamp bool, registryAuthfile string) ([]fn.Option, error) {
	if builder == "" {
		builder = builders.Pack
	}

	o := []fn.Option{
		fn.WithVerbose(cfg.Verbose),
	}

	t := fnhttp.NewRoundTripper(
		fnhttp.WithInsecureSkipVerify(cfg.InsecureSkipVerify),
		fnhttp.WithOpenShiftServiceCA(),
	)
	credsProvider := newCredentialsProvider(config.Dir(), t, registryAuthfile, cfg.InsecureSkipVerify)

	switch builder {
	case builders.Host:
		o = append(o,
			fn.WithScaffolder(oci.NewScaffolder(cfg.Verbose)),
			fn.WithBuilder(oci.NewBuilder(builders.Host, cfg.Verbose)),
			fn.WithPusher(oci.NewPusher(cfg.InsecureSkipVerify, false, cfg.Verbose,
				oci.WithTransport(fnhttp.NewRoundTripper(fnhttp.WithInsecureSkipVerify(cfg.InsecureSkipVerify), fnhttp.WithOpenShiftServiceCA())),
				oci.WithCredentialsProvider(credsProvider),
				oci.WithVerbose(cfg.Verbose))),
		)
	case builders.Pack:
		o = append(o,
			fn.WithScaffolder(buildpacks.NewScaffolder(cfg.Verbose)),
			fn.WithBuilder(buildpacks.NewBuilder(
				buildpacks.WithName(builders.Pack),
				buildpacks.WithTimestamp(withTimestamp),
				buildpacks.WithVerbose(cfg.Verbose))),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(credsProvider),
				docker.WithTransport(t),
				docker.WithVerbose(cfg.Verbose),
				docker.WithInsecure(cfg.InsecureSkipVerify))),
		)
	case builders.S2I:
		o = append(o,
			fn.WithScaffolder(s2i.NewScaffolder(cfg.Verbose)),
			fn.WithBuilder(s2i.NewBuilder(
				s2i.WithName(builders.S2I),
				s2i.WithVerbose(cfg.Verbose))),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(credsProvider),
				docker.WithTransport(t),
				docker.WithVerbose(cfg.Verbose),
				docker.WithInsecure(cfg.InsecureSkipVerify))),
		)
	default:
		return o, builders.ErrUnknownBuilder{Name: builder, Known: builders.All()}
	}
	return o, nil
}

// deployClientOptions returns client options for a deploy operation.
func deployClientOptions(builder, deployer string, cfg ClientConfig, withTimestamp bool) ([]fn.Option, error) {
	o, err := builderClientOptions(builder, cfg, withTimestamp, "")
	if err != nil {
		return nil, err
	}

	t := fnhttp.NewRoundTripper(
		fnhttp.WithInsecureSkipVerify(cfg.InsecureSkipVerify),
		fnhttp.WithOpenShiftServiceCA(),
	)
	credsProvider := newCredentialsProvider(config.Dir(), t, "", cfg.InsecureSkipVerify)

	o = append(o, fn.WithPipelinesProvider(tekton.NewPipelinesProvider(
		tekton.WithCredentialsProvider(credsProvider),
		tekton.WithVerbose(cfg.Verbose),
		tekton.WithTransport(t),
	)))

	if deployer == "" {
		deployer = knative.KnativeDeployerName
	}
	switch deployer {
	case knative.KnativeDeployerName:
		o = append(o, fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(cfg.Verbose))))
	case k8s.KubernetesDeployerName:
		o = append(o, fn.WithDeployer(k8s.NewDeployer(k8s.WithDeployerVerbose(cfg.Verbose))))
	case keda.KedaDeployerName:
		o = append(o, fn.WithDeployer(keda.NewDeployer(keda.WithDeployerVerbose(cfg.Verbose))))
	default:
		return nil, fmt.Errorf("unsupported deployer: %s (supported: %s, %s, %s)",
			deployer, knative.KnativeDeployerName, k8s.KubernetesDeployerName, keda.KedaDeployerName)
	}
	return o, nil
}

func platformBuildOptions(platform string) ([]fn.BuildOption, error) {
	if platform == "" {
		return nil, nil
	}
	parts := strings.Split(platform, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("platform must be in the form OS/Architecture (e.g. linux/amd64)")
	}
	return []fn.BuildOption{fn.BuildWithPlatforms([]fn.Platform{{OS: parts[0], Architecture: parts[1]}})}, nil
}

func shouldBuild(buildFlag string, f fn.Function) (bool, error) {
	if buildFlag == "" || buildFlag == "auto" {
		if f.Built() {
			return false, nil
		}
		return true, nil
	}
	build, err := parseBoolFlag(buildFlag)
	if err != nil {
		return false, fmt.Errorf("invalid build flag %q: must be 'auto' or a boolean", buildFlag)
	}
	return build, nil
}

func parseBoolFlag(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "true", "1", "t", "yes":
		return true, nil
	case "false", "0", "f", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", v)
	}
}

func isDigestedImage(v string) (bool, error) {
	ref, err := name.ParseReference(v)
	if err != nil {
		return false, err
	}
	_, ok := ref.(name.Digest)
	return ok, nil
}

func newCredentialsProvider(configPath string, t fnhttp.RoundTripCloser, authFilePath string, insecure bool) oci.CredentialsProvider {
	additionalLoaders := append(k8s.GetOpenShiftDockerCredentialLoaders(), k8s.GetGoogleCredentialLoader()...)
	additionalLoaders = append(additionalLoaders, k8s.GetECRCredentialLoader()...)
	additionalLoaders = append(additionalLoaders, k8s.GetACRCredentialLoader()...)

	options := []creds.Opt{
		creds.WithTransport(t),
		creds.WithInsecure(insecure),
		creds.WithAdditionalCredentialLoaders(additionalLoaders...),
	}
	if authFilePath != "" {
		options = append(options, creds.WithAuthFilePath(authFilePath))
	}
	return creds.NewCredentialsProvider(configPath, options...)
}
