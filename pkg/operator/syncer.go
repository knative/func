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
	repoURL, repoRevision, repoDir, err := source(f)
	if err != nil {
		return nil
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

// source returns the repository URL, revision and directory of f's source
// code, which the Function CR points the operator at.
//
// A function deployed by reference (--source) has no Root: its source is all
// there is, and an empty revision means the repository's default branch. Only
// a local function falls back to its git checkout, as an empty path would
// read the git repository of the current directory.
func source(f fn.Function) (url, revision, dir string, err error) {
	url, revision, dir = f.Build.Source.URL, f.Build.Source.Revision, f.Build.Source.Dir

	if f.Root != "" {
		if url == "" {
			if url, err = funcgit.ResolveRemoteURL(f.Root); err != nil {
				return
			}
		}
		if revision == "" {
			revision = funcgit.ResolveBranch(f.Root)
		}
	}

	if dir == "" {
		dir = "."
	}
	return
}
