package function

import (
	"context"
	"path"
)

// template
type template struct {
	name        string
	runtime     string
	repository  string
	fs          Filesystem
	templConfig templateConfig
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

func (t template) Write(ctx context.Context, f *Function) error {

	// Apply fields from the template onto the function itself (Denormalize).
	// The template is already the denormalized view of repo->runtime->template
	// so it's values are treated as defaults.
	// TODO: this begs the question: should the Template's manifest.yaml actually
	// be a partially-populated func.yaml?
	if f.Builder == "" { // as a special first case, this default comes from itself
		f.Builder = f.Builders["default"]
		if f.Builder == "" { // still nothing?  then use the template
			f.Builder = t.templConfig.Builders["default"]
		}
	}
	if len(f.Builders) == 0 {
		f.Builders = t.templConfig.Builders
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = t.templConfig.Buildpacks
	}
	if len(f.BuildEnvs) == 0 {
		f.BuildEnvs = t.templConfig.BuildEnvs
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = t.templConfig.HealthEndpoints.Liveness
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = t.templConfig.HealthEndpoints.Readiness
	}
	if f.Invocation.Format == "" {
		f.Invocation.Format = t.templConfig.Invocation.Format
	}

	isManifest := func(p string) bool {
		_, f := path.Split(p)
		return f == templateManifest
	}

	return copyFromFS(".", f.Root, maskingFS{fs: t.fs, masked: isManifest}) // copy everything but manifest.yaml
}
