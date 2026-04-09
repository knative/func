package buildpacks

import (
	"context"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"

	"knative.dev/func/pkg/docker"
)

// BuilderName is the short name of this builder.
const BuilderName = "pack"

// Register adds the Pack builder to the given registry.
func Register(r *fn.Registry) {
	r.RegisterBuilder(BuilderName, packFactory)
}

func packFactory(cfg fn.BuilderConfig) []fn.Option {
	creds := adapterCredentials(cfg.Credentials)
	return []fn.Option{
		fn.WithScaffolder(NewScaffolder(cfg.Verbose)),
		fn.WithBuilder(NewBuilder(
			WithName(BuilderName),
			WithTimestamp(cfg.WithTimestamp),
			WithVerbose(cfg.Verbose),
		)),
		fn.WithPusher(docker.NewPusher(
			docker.WithCredentialsProvider(creds),
			docker.WithTransport(cfg.Transport),
			docker.WithVerbose(cfg.Verbose),
		)),
	}
}

// adapterCredentials converts fn.CredentialsCallback to oci.CredentialsProvider.
func adapterCredentials(cb fn.CredentialsCallback) oci.CredentialsProvider {
	if cb == nil {
		return oci.EmptyCredentialsProvider
	}
	return func(ctx context.Context, image string) (oci.Credentials, error) {
		u, p, err := cb(ctx, image)
		return oci.Credentials{Username: u, Password: p}, err
	}
}
