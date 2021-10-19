package function

// Updating Templates:
// See documentation in ./templates/README.md
// go get github.com/markbates/pkger
//go:generate pkger

import (
	"errors"
	"path/filepath"
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
				names = append(names, t.Name)
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

// Write a template to disk for the given Function
// Returns a Function which may have been modified dependent on the content
// of the template (which can define default Function fields, builders,
// buildpacks, etc)
func (t *Templates) Write(f Function) (Function, error) {
	// The Function's Template
	template, err := t.Get(f.Runtime, f.Template)
	if err != nil {
		return f, err
	}

	// The Function's Template Repository
	repo, err := t.client.Repositories().Get(template.Repository)
	if err != nil {
		return f, err
	}

	// Validate paths:  (repo/)[templates/]<runtime>/<template>
	templatesPath := repo.TemplatesPath
	if _, err := repo.FS.Stat(templatesPath); err != nil {
		return f, ErrTemplatesNotFound
	}
	runtimePath := filepath.Join(templatesPath, template.Runtime)
	if _, err := repo.FS.Stat(runtimePath); err != nil {
		return f, ErrRuntimeNotFound
	}
	templatePath := filepath.Join(runtimePath, template.Name)
	if _, err := repo.FS.Stat(templatePath); err != nil {
		return f, ErrTemplateNotFound
	}

	// Apply fields from the template onto the function itself (Denormalize).
	// The template is already the denormalized view of repo->runtime->template
	// so it's values are treated as defaults.
	// TODO: this begs the question: should the Template's manifest.yaml actually
	// be a partially-populated func.yaml?
	if f.Builder == "" { // as a special fist case, this default comes from itself
		f.Builder = f.Builders["default"]
		if f.Builder == "" { // still nothing?  then use the template
			f.Builder = template.Builders["default"]
		}
	}
	if len(f.Builders) == 0 {
		f.Builders = template.Builders
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = template.Buildpacks
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = template.HealthEndpoints.Liveness
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = template.HealthEndpoints.Readiness
	}

	// Copy the template files from the repo filesystem to the new Function's root
	return f, copy(templatePath, f.Root, repo.FS)
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
