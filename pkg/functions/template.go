package functions

import (
	"context"
	"path"

	"knative.dev/func/pkg/filesystem"
)

// Template is a function project template.
// It can be used to instantiate new function project.
type Template interface {
	// Name of this template.
	Name() string
	// Runtime for which this template applies.
	Runtime() string
	// Repository within which this template is contained.  Value is set to the
	// currently effective name of the repository, which may vary. It is user-
	// defined when the repository is added, and can be set to "default" when
	// the client is loaded in single repo mode. I.e. not canonical.
	Repository() string
	// Fullname is a calculated field of [repo]/[name] used
	// to uniquely reference a template which may share a name
	// with one in another repository.
	Fullname() string
	// Write updates fields of function f and writes project files to path pointed by f.Root.
	Write(ctx context.Context, f *Function) error
}

// template default implementation
type template struct {
	name       string
	runtime    string
	repository string
	fs         filesystem.Filesystem
	config     templateConfig
}

func (t template) Name() string {
	return t.name
}

func (t template) Runtime() string {
	return t.runtime
}

func (t template) Repository() string {
	return t.repository
}

func (t template) Fullname() string {
	return t.repository + "/" + t.name
}

// Write the template source files
// (all source code except manifest.yaml and scaffolding)
func (t template) Write(ctx context.Context, f *Function) error {

	// Apply fields from the template onto the function itself (Denormalize).
	// The template is already the denormalized view of repo->runtime->template
	// so it's values are treated as defaults.
	// TODO: this begs the question: should the Template's manifest.yaml actually
	// be a partially-populated func.yaml?
	if len(f.Build.BuilderImages) == 0 {
		f.Build.BuilderImages = t.config.BuilderImages
	}
	if len(f.Build.Buildpacks) == 0 {
		f.Build.Buildpacks = t.config.Buildpacks
	}
	if len(f.Build.BuildEnvs) == 0 {
		f.Build.BuildEnvs = t.config.BuildEnvs
	}
	if len(f.Run.Envs) == 0 {
		f.Run.Envs = t.config.RunEnvs
	}
	if f.Deploy.HealthEndpoints.Liveness == "" {
		f.Deploy.HealthEndpoints.Liveness = t.config.HealthEndpoints.Liveness
	}
	if f.Deploy.HealthEndpoints.Readiness == "" {
		f.Deploy.HealthEndpoints.Readiness = t.config.HealthEndpoints.Readiness
	}
	if f.Invoke == "" && t.config.Invoke != "http" {
		f.Invoke = t.config.Invoke
	}

	mask := func(p string) bool {
		_, f := path.Split(p)
		return f == templateManifest
	}

	return filesystem.CopyFromFS(".", f.Root, filesystem.NewMaskingFS(mask, t.fs)) // copy everything but manifest.yaml
}
