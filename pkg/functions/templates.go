package functions

import (
	"context"
	"strings"

	"knative.dev/func/pkg/utils"
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
	extended := utils.NewSortedSet()

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
		repo     Repository
		err      error
	)
	repoName, tplName := splitTemplateFullname(fullname)

	// Get specified repository
	repo, err = t.client.Repositories().Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.Template(runtime, tplName)
}

// splits a template reference into its constituent parts: repository name
// and template name.
// The form '[repo]/[name]'.  The reposititory name and slash prefix are
// optional, in which case DefaultRepositoryName is returned.
func splitTemplateFullname(name string) (repoName, tplName string) {
	// Split into repo and template names.
	// Defaults when unprefixed to DefaultRepositoryName
	cc := strings.Split(name, "/")
	if len(cc) == 1 {
		repoName = DefaultRepositoryName
		tplName = name
	} else {
		repoName = cc[0]
		tplName = cc[1]
	}
	return
}

// Write a function's template to disk.
// Returns a function which may have been modified dependent on the content
// of the template (which can define default function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f *Function) error {
	// Ensure the function itself is syntactically valid
	if err := f.Validate(); err != nil {
		return err
	}

	// The function's Template
	template, err := t.Get(f.Runtime, f.Template)
	if err != nil {
		return err
	}

	return template.Write(context.TODO(), f)
}
