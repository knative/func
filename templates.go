package function

// Updating Templates:
// See documentation in ./templates/README.md
// go get github.com/markbates/pkger
//go:generate pkger

import (
	"context"
	"errors"
	"strings"

	"github.com/markbates/pkger"
)

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrTemplatesNotFound         = errors.New("templates path (runtimes) not found")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

// Templates Manager
type Templates struct {
	repositories *Repositories
}

// newTemplates manager
// Includes a back-reference to client (logic tree root) such
// that the templates manager has full access to the API for
// use in its implementations.
func newTemplates(repositories *Repositories) *Templates {
	return &Templates{repositories: repositories}
}

// List the full name of templates available for the runtime.
// Full name is the optional repository prefix plus the template's repository
// local name.  Default templates grouped first sans prefix.
func (t *Templates) List(runtime string) ([]string, error) {
	names := []string{}
	extended := newSortedSet()

	rr, err := t.repositories.All()
	if err != nil {
		return []string{}, err
	}

	for _, r := range rr {
		tt, err := r.Templates(context.TODO(), runtime)
		if err != nil {
			return []string{}, err
		}
		for _, t := range tt {
			if r.Name() == DefaultRepositoryName {
				names = append(names, t)
			} else {
				extended.Add(r.Name() + "/" + t)
			}
		}
	}
	return append(names, extended.Items()...), nil
}

// Template returns the named template in full form '[repo]/[name]' for the
// specified runtime.
// Templates from the default repository do not require the repo name prefix,
// though it can be provided.
func (t *Templates) Get(runtime, fullname string) (ITemplate, error) {
	var (
		template Template
		repoName string
		tplName  string
		repo     IRepository
		err      error
	)

	// Split into repo and template names.
	// Defaults when unprefixed to DefaultRepository
	cc := strings.Split(fullname, "/")
	if len(cc) == 1 {
		repoName = DefaultRepositoryName
		tplName = fullname
	} else {
		repoName = cc[0]
		tplName = cc[1]
	}

	// Get specified repository
	repo, err = t.repositories.Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.Template(context.TODO(), runtime, tplName)
}

// Write a template to disk for the given Function
// Returns a Function which may have been modified dependent on the content
// of the template (which can define default Function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f Function) error {
	// The Function's Template
	template, err := t.Get(f.Runtime, f.Template)
	if err != nil {
		return err
	}

	return template.Write(context.TODO(), f.Name, f.Root)
}

// Embedding Directives
// Trigger encoding of ./templates as pkged.go

// Path to embedded
// note: this constant must be defined in the file in which pkger is called,
// as it performs static analysis on each source file separately to trigger
// encoding of referenced paths.
const embeddedPath = "/templates"

// When pkger is run, code analysis detects this pkger.Include statement,
// triggering the serialization of the templates directory and all its contents
// into pkged.go, which is then made available via a pkger filesystem.  Path is
// relative to the go module root.
func init() {
	_ = pkger.Include(embeddedPath)
}
