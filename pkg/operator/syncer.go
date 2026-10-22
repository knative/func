package operator

import (
	"context"

	fn "knative.dev/func/pkg/functions"

	"knative.dev/func/pkg/docker"
	funcgit "knative.dev/func/pkg/git"
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

	var registryCredentials *RegistryCredentials
	if s.credentialsProvider != nil && f.Deploy.Image != "" {
		registry, err := docker.GetRegistry(f.Deploy.Image)
		if err == nil {
			creds, err := s.credentialsProvider(ctx, f.Deploy.Image)
			if err == nil && creds.Username != "" {
				registryCredentials = &RegistryCredentials{
					Username: creds.Username,
					Password: creds.Password,
					Server:   registry,
				}
			}
		}
	}

	return SyncFunctionCR(ctx, SyncConfig{
		FunctionName:        f.Name,
		Namespace:           namespace,
		RepoURL:             repoURL,
		RepoRevision:        repoRevision,
		RepoDir:             repoDir,
		RegistryCredentials: registryCredentials,
	})
}
