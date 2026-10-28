package operator

import (
	"context"

	v1 "k8s.io/api/core/v1"

	fn "knative.dev/func/pkg/functions"

	"knative.dev/func/pkg/docker"
	funcgit "knative.dev/func/pkg/git"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/oci"
)

type SyncerOpt func(*Syncer)

type Syncer struct {
	credentialsProvider oci.CredentialsProvider
}

func NewSyncer(opts ...SyncerOpt) *Syncer {
	s := &Syncer{}
	for _, o := range opts {
		o(s)
	}
	return s
}

func WithCredentialsProvider(cp oci.CredentialsProvider) SyncerOpt {
	return func(s *Syncer) {
		s.credentialsProvider = cp
	}
}

func (s *Syncer) Sync(ctx context.Context, f fn.Function) error {
	repoURL := f.Build.Source.URL
	repoRevision := f.Build.Source.Revision
	repoDir := f.Build.Source.Dir

	if repoURL == "" {
		resolved, err := funcgit.ResolveRemoteURL(f.Root)
		if err != nil {
			return nil
		}
		repoURL = resolved
	}

	if repoRevision == "" {
		repoRevision = funcgit.ResolveBranch(f.Root)
	}

	if repoDir == "" {
		repoDir = "."
	}

	namespace := f.Deploy.Namespace
	if namespace == "" {
		namespace = f.Namespace
	}

	registrySecret, err := s.registrySecret(ctx, f)
	if err != nil {
		return err
	}

	return SyncFunctionCR(ctx, SyncConfig{
		FunctionName:   f.Name,
		Namespace:      namespace,
		RepoURL:        repoURL,
		RepoRevision:   repoRevision,
		RepoDir:        repoDir,
		RegistrySecret: registrySecret,
	})
}

// registrySecret returns the registry Secret of f, holding the login to the
// registry of its image, or nil when it finds no login.
func (s *Syncer) registrySecret(ctx context.Context, f fn.Function) (*v1.Secret, error) {
	if s.credentialsProvider == nil || f.Deploy.Image == "" {
		return nil, nil
	}
	registry, err := docker.GetRegistry(f.Deploy.Image)
	if err != nil {
		return nil, nil
	}
	creds, err := s.credentialsProvider(ctx, f.Deploy.Image)
	if err != nil || creds.Username == "" {
		return nil, nil
	}
	secret, err := k8s.NewRegistrySecret(f, creds.Username, creds.Password, registry)
	if err != nil {
		return nil, err
	}
	return &secret, nil
}
