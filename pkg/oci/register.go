package oci

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

// BuilderName is the short name of this builder.
const BuilderName = "host"

// Register adds the Host (OCI) builder to the given registry.
func Register(r *fn.Registry) {
	r.RegisterBuilder(BuilderName, hostFactory)
}

func hostFactory(cfg fn.BuilderConfig) []fn.Option {
	creds := adapterCredentials(cfg.Credentials)
	return []fn.Option{
		fn.WithScaffolder(NewScaffolder(cfg.Verbose)),
		fn.WithBuilder(NewBuilder(BuilderName, cfg.Verbose)),
		fn.WithPusher(NewPusher(cfg.RegistryInsecure, false, cfg.Verbose,
			WithCredentialsProvider(creds),
			WithTransport(cfg.Transport),
		)),
	}
}

// adapterCredentials converts fn.CredentialsCallback to oci.CredentialsProvider.
func adapterCredentials(cb fn.CredentialsCallback) CredentialsProvider {
	if cb == nil {
		return EmptyCredentialsProvider
	}
	return func(ctx context.Context, image string) (Credentials, error) {
		u, p, err := cb(ctx, image)
		return Credentials{Username: u, Password: p}, err
	}
}
