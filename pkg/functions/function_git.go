package functions

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	"gopkg.in/yaml.v2"
)

// NewFunctionFromGit loads the function committed in the repository g
// describes: the func.yaml in g.Dir at g.Revision, which is the remote's
// default branch when empty. A revision may be a branch, a tag or a full
// commit hash.
//
// No working copy is involved. The returned function therefore has no Root
// and none of the state NewFunction reads from one (local settings, last
// built image). Its Build.Source is g, where it was read from, with the
// commit g's revision resolved to: whatever source settings the committed
// func.yaml holds are replaced, so the commit is never paired with another
// repository.
func NewFunctionFromGit(ctx context.Context, g Source) (Function, error) {
	if errs := validateSource(g); len(errs) > 0 {
		return Function{}, errors.New(strings.Join(errs, "; "))
	}
	src, err := resolveGitSource(ctx, g)
	if err != nil {
		return Function{}, err
	}
	commit, err := src.fetch(ctx)
	if err != nil {
		return Function{}, err
	}
	// Read from the commit's tree: nothing is checked out.
	file, err := commit.File(strings.TrimPrefix(path.Join(g.Dir, FunctionFile), "/"))
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
			return Function{}, fmt.Errorf("no %s in %q of %s at %s", FunctionFile, g.Dir, g.URL, src.describe())
		}
		return Function{}, fmt.Errorf("cannot read %s from %s: %w", FunctionFile, g.URL, err)
	}
	contents, err := file.Contents()
	if err != nil {
		return Function{}, fmt.Errorf("cannot read %s from %s: %w", FunctionFile, g.URL, err)
	}
	bb := []byte(contents)
	// Stricter than NewFunction: a func.yaml which does not unmarshal cleanly
	// is an error even if it migrates.
	f := Function{}
	f.Build.BuilderImages = make(map[string]string)
	f.Deploy.Annotations = make(map[string]string)
	if err = yaml.Unmarshal(bb, &f); err != nil {
		return Function{}, formatUnmarshalError(err)
	}
	if f, err = f.migrate(bb); err != nil {
		return Function{}, err
	}
	// Where the function was read from, and the commit it was read at, for
	// the build to fetch and label.
	f.Build.Source = g
	f.Build.Source.Commit = commit.Hash.String()
	return f, nil
}

// gitSource is a revision of a remote repository, in the form the fetch
// needs it: a ref to clone, a commit to fetch, or neither for the remote's
// default branch.
type gitSource struct {
	url  string
	auth transport.AuthMethod
	// ref is the branch or tag to clone. Empty for the default branch, and
	// for a bare commit, which hash then names.
	ref  plumbing.ReferenceName
	hash plumbing.Hash
}

// resolveGitSource decides how g is fetched. An empty revision is the
// remote's default branch, a full ref name (refs/...) and a full commit hash
// are taken as given; none of these needs the remote's refs. A bare name does:
// it is matched against them as git matches one (see gitrevisions), a tag
// before a branch, so a tag wins over a branch of the same name as it does
// for git fetch. Listing the refs costs one round trip and, on large
// repositories, a sizeable advertisement, hence only for bare names.
func resolveGitSource(ctx context.Context, g Source) (gitSource, error) {
	src := gitSource{url: g.URL}
	if g.URL == "" {
		return src, errors.New("git URL required")
	}
	switch {
	case g.Revision == "":
		return src, nil
	case strings.HasPrefix(g.Revision, "refs/"):
		src.ref = plumbing.ReferenceName(g.Revision)
		return src, nil
	case plumbing.IsHash(g.Revision):
		src.hash = plumbing.NewHash(g.Revision)
		return src, nil
	}

	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{g.URL},
	})
	var refs []*plumbing.Reference
	err := withAuth(g.URL, &src.auth, func(auth transport.AuthMethod) (err error) {
		refs, err = remote.ListContext(ctx, &git.ListOptions{Auth: auth})
		return
	})
	if err != nil {
		return src, fmt.Errorf("cannot list refs of %s: %w", g.URL, err)
	}
	byName := make(map[plumbing.ReferenceName]bool, len(refs))
	for _, r := range refs {
		byName[r.Name()] = true
	}
	for _, name := range []plumbing.ReferenceName{
		plumbing.NewTagReferenceName(g.Revision),
		plumbing.NewBranchReferenceName(g.Revision),
	} {
		if byName[name] {
			src.ref = name
			return src, nil
		}
	}
	if isAbbreviatedHash(g.Revision) {
		// A remote cannot resolve an abbreviation: it serves objects by their
		// full id, and neither can the cluster's fetch.
		return src, fmt.Errorf("revision %q not found in %s: a commit must be given as its full hash", g.Revision, g.URL)
	}
	return src, fmt.Errorf("revision %q not found in %s", g.Revision, g.URL)
}

// withAuth runs op without credentials and, if the remote asked for
// authentication, once more with those the local git configuration holds
// for url. The credentials that worked are left in auth for later calls.
func withAuth(url string, auth *transport.AuthMethod, op func(transport.AuthMethod) error) error {
	err := op(*auth)
	if isAuthError(err) && *auth == nil {
		if a := credentialsForURL(url); a != nil {
			*auth = a
			err = op(a)
		}
	}
	return err
}

// isAbbreviatedHash reports whether s looks like a shortened commit hash.
func isAbbreviatedHash(s string) bool {
	if len(s) < 4 || len(s) >= 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// describe returns the revision for messages: the ref's short name, the
// commit hash, or HEAD for the remote's default branch.
func (s gitSource) describe() string {
	switch {
	case s.ref != "":
		return s.ref.Short()
	case !s.hash.IsZero():
		return s.hash.String()
	}
	return "HEAD"
}

// fetch fetches the resolved revision, depth one, into memory storage and
// returns its commit. There is no worktree: files are read from the
// commit's tree.
func (s *gitSource) fetch(ctx context.Context) (*object.Commit, error) {
	var repo *git.Repository
	err := withAuth(s.url, &s.auth, func(auth transport.AuthMethod) (err error) {
		if !s.hash.IsZero() {
			// A bare commit cannot be cloned: fetch it by hash.
			repo, err = fetchGitCommit(ctx, s.url, s.hash, auth)
			return
		}
		// An empty ReferenceName clones the remote's default branch. A nil
		// worktree makes the clone bare, so nothing is checked out.
		repo, err = git.CloneContext(ctx, memory.NewStorage(), nil, &git.CloneOptions{
			URL:               s.url,
			Auth:              auth,
			ReferenceName:     s.ref,
			SingleBranch:      true,
			Depth:             1,
			Tags:              git.NoTags,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
		return
	})
	if err != nil {
		return nil, fmt.Errorf("cannot fetch %s at %s: %w", s.url, s.describe(), err)
	}
	hash := s.hash
	if hash.IsZero() {
		head, err := repo.Head()
		if err != nil {
			return nil, err
		}
		hash = head.Hash()
	}
	return repo.CommitObject(hash)
}

func fetchGitCommit(ctx context.Context, url string, hash plumbing.Hash, auth transport.AuthMethod) (*git.Repository, error) {
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		return nil, err
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{url},
	})
	if err != nil {
		return nil, err
	}
	// Fetching a hash directly requires the server to allow it
	// (uploadpack.allowReachableSHA1InWant), as the common hosts do.
	err = remote.FetchContext(ctx, &git.FetchOptions{
		Auth:  auth,
		Depth: 1,
		Tags:  git.NoTags,
		RefSpecs: []config.RefSpec{
			config.RefSpec(hash.String() + ":" + plumbing.NewRemoteReferenceName(git.DefaultRemoteName, "source").String()),
		},
	})
	if err != nil {
		return nil, err
	}
	return repo, nil
}
