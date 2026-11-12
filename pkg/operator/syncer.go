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
	repoURL, repoRevision, repoDir, err := source(f)
	if err != nil {
		return nil
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

// Unsync deletes the Function CR of the function name in namespace.
func (s *Syncer) Unsync(ctx context.Context, name, namespace string) error {
	return RemoveFunctionCR(ctx, name, namespace)
}

// source returns the repository URL, revision and directory of f's source
// code, which the Function CR points the operator at. The URL never holds
// credentials: anyone who can read the CR can read it.
//
// It returns build.source as it is, and fills in what func.yaml leaves empty
// from the git checkout of a local function: the url, and the branch when the
// deploy built the checkout. A remote build of the url in func.yaml fetches
// that url, so an empty revision stays empty: the repository's default
// branch, as it is for the build. A checkout on no branch, such as a detached
// HEAD in CI, leaves it empty too. A function deployed by reference (--source)
// has no Root, so nothing is filled in: an empty path would read the git
// repository of the current directory.
func source(f fn.Function) (url, revision, dir string, err error) {
	url, revision, dir = f.Build.Source.URL, f.Build.Source.Revision, f.Build.Source.Dir

	// The cluster fetched the url, not the checkout.
	fetched := f.Local.Remote && url != ""

	if f.Root != "" {
		if url == "" {
			if url, err = funcgit.ResolveRemoteURL(f.Root); err != nil {
				return
			}
		}
		if revision == "" && !fetched {
			revision = funcgit.ResolveBranch(f.Root)
		}
	}

	url = funcgit.StripUserinfo(url)
	if dir == "" {
		dir = "."
	}
	return
}
