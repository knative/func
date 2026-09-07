package functions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Git struct {
	URL        string `yaml:"url,omitempty"`
	Revision   string `yaml:"revision,omitempty"`
	ContextDir string `yaml:"contextDir,omitempty"`
}

// validateGit validates input Git option from Function config
func validateGit(git Git) (errors []string) {
	if git.URL != "" {
		_, err := giturls.ParseTransport(git.URL)
		if err != nil {
			_, err = giturls.ParseScp(git.URL)
		}
		if err != nil {
			errMsg := fmt.Sprintf("specified option \"git.url=%s\" is not valid", git.URL)

			originalErr := err.Error()
			if !strings.HasSuffix(originalErr, "is not a valid transport") {
				errMsg = fmt.Sprintf("%s, error: %s", errMsg, originalErr)
			}
			errors = append(errors, errMsg)
		}
	}
	return
}

// NewFunctionFromGit loads the function committed in the repository g
// describes: the func.yaml in g.ContextDir at g.Revision, which is the
// remote's default branch when empty. A revision may be a branch, a tag or a
// full commit hash.
//
// No working copy is involved. The returned function therefore has no Root
// and none of the state NewFunction reads from one (local settings, last
// built image).
func NewFunctionFromGit(ctx context.Context, g Git) (Function, error) {
	src, err := resolveGitSource(ctx, g)
	if err != nil {
		return Function{}, err
	}
	tree, err := src.checkout(ctx)
	if err != nil {
		return Function{}, err
	}
	bb, err := util.ReadFile(tree, path.Join(g.ContextDir, FunctionFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Function{}, fmt.Errorf("no %s in %q of %s at %s", FunctionFile, g.ContextDir, g.URL, src.describe())
		}
		return Function{}, fmt.Errorf("cannot read %s from %s: %w", FunctionFile, g.URL, err)
	}
	return parseFunction(bb)
}

// gitSource is a revision of a remote repository, resolved against the refs
// the remote advertises.
type gitSource struct {
	url  string
	auth transport.AuthMethod
	// ref is the branch or tag to fetch. It is empty when the revision was
	// given as a bare commit hash.
	ref plumbing.ReferenceName
	// hash is the commit the revision resolves to.
	hash plumbing.Hash
}

// resolveGitSource lists the refs of g.URL and matches g.Revision against
// them. An empty revision means the remote's default branch. Otherwise a
// branch, a tag, a full ref name and a full commit hash are tried, in that
// order.
func resolveGitSource(ctx context.Context, g Git) (gitSource, error) {
	src := gitSource{url: g.URL}
	if g.URL == "" {
		return src, errors.New("git URL required")
	}

	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{g.URL},
	})
	opts := &git.ListOptions{PeelingOption: git.AppendPeeled}
	refs, err := remote.ListContext(ctx, opts)
	if isAuthError(err) {
		if src.auth = credentialsForURL(g.URL); src.auth != nil {
			opts.Auth = src.auth
			refs, err = remote.ListContext(ctx, opts)
		}
	}
	if err != nil {
		return src, fmt.Errorf("cannot list refs of %s: %w", g.URL, err)
	}

	byName := make(map[plumbing.ReferenceName]*plumbing.Reference, len(refs))
	for _, r := range refs {
		byName[r.Name()] = r
	}
	// commitOf returns the commit a ref points to: the peeled object for an
	// annotated tag, the ref's own hash otherwise.
	commitOf := func(name plumbing.ReferenceName) plumbing.Hash {
		if peeled, ok := byName[name+"^{}"]; ok {
			return peeled.Hash()
		}
		return byName[name].Hash()
	}

	if g.Revision == "" {
		head, ok := byName[plumbing.HEAD]
		if !ok {
			return src, fmt.Errorf("%s advertises no HEAD; specify a revision", g.URL)
		}
		if head.Type() != plumbing.SymbolicReference {
			src.hash = head.Hash()
			return src, nil
		}
		if _, ok := byName[head.Target()]; !ok {
			return src, fmt.Errorf("%s: HEAD points to %s, which the remote does not advertise", g.URL, head.Target())
		}
		src.ref = head.Target()
		src.hash = commitOf(src.ref)
		return src, nil
	}

	for _, name := range []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName(g.Revision),
		plumbing.NewTagReferenceName(g.Revision),
		plumbing.ReferenceName(g.Revision),
	} {
		if _, ok := byName[name]; ok {
			src.ref = name
			src.hash = commitOf(name)
			return src, nil
		}
	}
	if plumbing.IsHash(g.Revision) {
		src.hash = plumbing.NewHash(g.Revision)
		return src, nil
	}
	return src, fmt.Errorf("revision %q not found in %s", g.Revision, g.URL)
}

// describe returns the revision for messages: the ref's short name when the
// revision named one, the commit hash otherwise.
func (s gitSource) describe() string {
	if s.ref != "" {
		return s.ref.Short()
	}
	return s.hash.String()
}

// checkout fetches the resolved revision, depth one, into memory and returns
// its tree.
func (s gitSource) checkout(ctx context.Context) (billy.Filesystem, error) {
	var (
		repo *git.Repository
		err  error
	)
	if s.ref != "" {
		repo, err = git.CloneContext(ctx, memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL:               s.url,
			Auth:              s.auth,
			ReferenceName:     s.ref,
			SingleBranch:      true,
			Depth:             1,
			Tags:              git.NoTags,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
	} else {
		// A bare commit cannot be cloned: fetch it by hash and check it out.
		repo, err = fetchGitCommit(ctx, s)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot fetch %s at %s: %w", s.url, s.describe(), err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	return wt.Filesystem, nil
}

func fetchGitCommit(ctx context.Context, s gitSource) (*git.Repository, error) {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		return nil, err
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{s.url},
	})
	if err != nil {
		return nil, err
	}
	// Fetching a hash directly requires the server to allow it
	// (uploadpack.allowReachableSHA1InWant), as the common hosts do.
	err = remote.FetchContext(ctx, &git.FetchOptions{
		Auth:  s.auth,
		Depth: 1,
		Tags:  git.NoTags,
		RefSpecs: []config.RefSpec{
			config.RefSpec(s.hash.String() + ":" + plumbing.NewRemoteReferenceName(git.DefaultRemoteName, "source").String()),
		},
	})
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	if err = wt.Checkout(&git.CheckoutOptions{Hash: s.hash}); err != nil {
		return nil, err
	}
	return repo, nil
}
