package function

// Updating Templates:
// See documentation in ./templates/README.md
// go get github.com/markbates/pkger
//go:generate pkger

import (
	"strings"
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
	// TODO: if repository override was enabled, we should just return those, flat.
	builtin, err := t.listDefault(runtime)
	if err != nil {
		return []string{}, err
	}

	extended, err := t.ListExtended(runtime)
	if err != nil && err != ErrTemplateNotFound {
		return []string{}, err
	}

	// Result is an alphanumerically sorted list first grouped by
	// embedded at head.
	return append(builtin, extended...), nil
}

// listDefault (embedded) templates by runtime
func (t *Templates) listDefault(runtime string) ([]string, error) {
	names := newSortedSet()

	r, err := t.client.Repositories().Get(DefaultRepository)
	if err != nil {
		return []string{}, err
	}

	tt, err := r.Templates(runtime)
	if err != nil {
		return []string{}, err
	}

	for _, t := range tt {
		names.Add(t.Name)
	}
	return names.Items(), nil
}

// listExtended templates returns all template full names that
// exist in all extended (config dir) repositories for a runtime.
// Prefixed, sorted.
func (t *Templates) listExtended(runtime string) ([]string, error) {
	names := newSortedSet()

	rr, err := t.client.Repositories().All()
	if err != nil {
		return []string{}, err
	}

	for _, r := range rr {
		if r.Name == DefaultRepository {
			continue // already added at head of names
		}
		tt, err := r.Templates(runtime)
		if err != nil {
			return []string{}, err
		}
		for _, t := range tt {
			names.Add(t.Fullname())
		}
	}
	return names.Items(), nil
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
		repo     Repository0_18
		err      error
	)

	// Split into repo and template names.
	// Defaults when unprefixed to DefaultRepository
	cc := strings.Split(fullname, "/")
	if len(cc) == 1 {
		repoName = DefaultRepository
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

// Write a Function to disk using the named Function at the given location
// Returns a new Function which may have been modified dependent on the content
// of the template (which can define default Function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f Function) (Function, error) {

	template, err := t.Get(f.Runtime, f.Template)
	if err != nil {
		return f, err
	}

	// Denormalize
	// Takes fields from the repo/runtime/template and sets them on the Function
	// if they're not already defined.
	// (builders, buildpacks, health endpoints)
	f, err = denormalize(t.client, template, f)
	if err != nil {
		return f, err
	}

	// write template from repositories path to the function root.
	return f, writeTemplate(template, t.client.Repositories().Path(), f.Root)
}

// denormalize fields from repo/runtime/template into fields on the Function
func denormalize(client *Client, t Template, f Function) (Function, error) {
	// The template is already the denormalized view of repo->runtime->template
	// so it's values are treated as defaults.
	//
	// This denormalizaiton might fit more conceptually correctly in a special
	// purpose Function constructor; but this is closer to the goal.  The template
	// is a hierarchically-derived set of attributes, allowing for the us to write
	// based on the calculated fields, reuturning the final Function as the
	// serialized, denormalized final data structure.  This separates the somewhat
	// complex hierarchical derivation of these manifests from the somewhat
	// orthoganal task of applying the values to a Function prior to writing it
	// out to disk.

	if f.Builder == "" { // as a special fist case, this default comes from itself
		f.Builder = f.Builders["default"]
		if f.Builder == "" { // still nothing?  then use the template
			f.Builder = t.Builders["default"]
		}
	}
	if len(f.Builders) == 0 {
		f.Builders = t.Builders
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = t.Buildpacks
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = t.HealthEndpoints.Liveness
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = t.HealthEndpoints.Readiness
	}
	return f, nil
}
