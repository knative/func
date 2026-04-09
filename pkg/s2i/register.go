package s2i

import (
	"context"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"

	"knative.dev/func/pkg/docker"
)

// BuilderName is the short name of this builder.
const BuilderName = "s2i"

// Register adds the S2I builder to the given registry.
func Register(r *fn.Registry) {
	r.RegisterBuilder(BuilderName, s2iFactory)
}

func s2iFactory(cfg fn.BuilderConfig) []fn.Option {
	creds := adapterCredentials(cfg.Credentials)
	return []fn.Option{
		fn.WithScaffolder(NewScaffolder(cfg.Verbose)),
		fn.WithBuilder(NewBuilder(
			WithName(BuilderName),
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
