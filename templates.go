package function

import (
	"context"
	"errors"
	"strings"
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
	client *Client
}

// newTemplates manager
// Includes a back-reference to client (logic tree root) such
// that the templates manager has full access to the API for
// use in its implementations.
func newTemplates(client *Client) *Templates {
	return &Templates{client: client}
}

// List the full name of templates available for the runtime.
// Full name is the optional repository prefix plus the template's repository
// local name.  Default templates grouped first sans prefix.
func (t *Templates) List(runtime string) ([]string, error) {
	names := []string{}
	extended := newSortedSet()

	rr, err := t.client.Repositories().All()
	if err != nil {
		return []string{}, err
	}

	for _, r := range rr {
		tt, err := r.Templates(runtime)
		if err != nil {
			return []string{}, err
		}
		for _, t := range tt {
			if r.Name == DefaultRepositoryName {
				names = append(names, t.Name())
			} else {
				extended.Add(t.Fullname())
			}
		}
	}
	return append(names, extended.Items()...), nil
}

// Template returns the named template in full form '[repo]/[name]' for the
// specified runtime.
// Templates from the default repository do not require the repo name prefix,
// though it can be provided.
func (t *Templates) Get(runtime, fullname string) (Template, error) {
	var (
		template Template
		repoName string
		tplName  string
		repo     Repository
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
	repo, err = t.client.Repositories().Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.Template(runtime, tplName)
}

// Write a function's template to disk.
// Returns a Function which may have been modified dependent on the content
// of the template (which can define default Function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f *Function) error {
	// Templates require an initially valid Function to write
	// (has name, path, runtime etc)
	if err := f.Validate(); err != nil {
		return err
	}

	// The Function's Template
	template, err := t.Get(f.Runtime, f.Template)
	if err != nil {
		return err
	}

	return template.Write(context.TODO(), f)
}
