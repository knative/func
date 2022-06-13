package function

import (
	"context"
	"path"
)

// template
type template struct {
	name       string
	runtime    string
	repository string
	fs         Filesystem
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

func (t template) Write(ctx context.Context, f *Function) error {

	// Apply fields from the template onto the function itself (Denormalize).
	// The template is already the denormalized view of repo->runtime->template
	// so it's values are treated as defaults.
	// TODO: this begs the question: should the Template's manifest.yaml actually
	// be a partially-populated func.yaml?
	if len(f.BuilderImages) == 0 {
		f.BuilderImages = t.config.BuilderImages
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = t.config.Buildpacks
	}
	if len(f.BuildEnvs) == 0 {
		f.BuildEnvs = t.config.BuildEnvs
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = t.config.HealthEndpoints.Liveness
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = t.config.HealthEndpoints.Readiness
	}
	if f.Invocation.Format == "" {
		f.Invocation.Format = t.config.Invocation.Format
	}

	isManifest := func(p string) bool {
		_, f := path.Split(p)
		return f == templateManifest
	}

	return copyFromFS(".", f.Root, maskingFS{fs: t.fs, masked: isManifest}) // copy everything but manifest.yaml
}
