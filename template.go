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
	if len(f.Build.BuilderImages) == 0 {
		f.Build.BuilderImages = t.config.BuilderImages
	}
	if len(f.Build.Buildpacks) == 0 {
		f.Build.Buildpacks = t.config.Buildpacks
	}
	if len(f.Build.BuildEnvs) == 0 {
		f.Build.BuildEnvs = t.config.BuildEnvs
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

	isManifest := func(p string) bool {
		_, f := path.Split(p)
		return f == templateManifest
	}

	return copyFromFS(".", f.Root, maskingFS{fs: t.fs, masked: isManifest}) // copy everything but manifest.yaml
}
