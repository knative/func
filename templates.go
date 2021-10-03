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
	var (
		names     = newSortedSet()
		repo, err = t.client.Repositories().Get(DefaultRepository)
		templates FunctionTemplates
	)
	if err != nil {
		return []string{}, err
	}

	if templates, err = repo.Templates(runtime); err != nil {
		return []string{}, err
	}
	for _, t := range templates {
		names.Add(t.Name)
	}
	return names.Items(), nil
}

// listExtended templates returns all template full names that
// exist in all extended (config dir) repositories for a runtime.
// Prefixed, sorted.
func (t *Templates) listExtended(runtime string) ([]string, error) {
	var (
		names      = newSortedSet()
		repos, err = t.client.Repositories().All()
		templates  FunctionTemplates
	)
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repos {
		if repo.Name == DefaultRepository {
			continue // already added at head of names
		}
		if templates, err = repo.Templates(runtime); err != nil {
			return []string{}, err
		}
		for _, template := range templates {
			names.Add(Template{
				Name:       template.Path,
				Repository: repo.Name,
				Runtime:    runtime,
			}.Fullname())
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
		repo     Repository
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

// Write a Function disk using the named Function at the given location
// Returns a new Function which may have been modified dependent on the content
// of the template (which can define default Function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f Function) (Function, error) {
	// TODO: These defaults are in the wrong place and will need to move to
	// (most likely) the Function constructor, which defines the template name,
	// runtime and repository to use.
	if f.Template == "" {
		f.Template = DefaultTemplate
	}
	if f.Runtime == "" {
		f.Runtime = DefaultRuntime
	}

	// Fetch the template instance for this Function
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

	// write template to path potentially using the given template repositories.
	return f, writeTemplate(template, f.Root, t.client.Repositories().Path())
}

// denormalize fields from repo/runtime/template into fields on the Function
func denormalize(client *Client, t Template, f Function) (Function, error) {
	// TODO: this denormalizaiton might better be part of either the Template
	// or Function instantiation process; a hierarchically-derived set of
	// attributes on the template itself, allowing for the template to simply
	// write based on the calculated fields upon its inception, reuturning the
	// final Function as the serialized, denormalized data structure.  This would
	// separate the somewhat complex hierarchical derivation of these fields from
	// the somewhat orthoganal task of writing.
	repo, err := client.Repositories().Get(t.Repository)
	if err != nil {
		return f, err
	}
	runtime, err := repo.Runtime(f.Runtime)
	if err != nil {
		return f, err
	}

	if f.Builder == "" {
		f.Builder = runtime.Builders["default"]
	}
	if len(f.Builders) == 0 {
		f.Builders = runtime.Builders
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = runtime.Buildpacks
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = repo.HealthEndpoints.Liveness
		if f.HealthEndpoints.Liveness == "" {
			f.HealthEndpoints.Liveness = runtime.Liveness
		}
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = repo.HealthEndpoints.Readiness
		if f.HealthEndpoints.Readiness == "" {
			f.HealthEndpoints.Readiness = runtime.Readiness
		}
	}
	return f, nil
}
