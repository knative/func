package function

import (
	"context"
	"path"
)

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
	// Write updates fields of Function f and writes project files to path pointed by f.Root.
	Write(ctx context.Context, f *Function) error
}

type templateConfig struct {
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`

	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`

	// BuildEnvs defines environment variables related to the builders,
	// this can be used to parameterize the builders
	BuildEnvs []Env `yaml:"buildEnvs,omitempty"`

	// Invocation defines invocation hints for a Functions which is created
	// from this template prior to being materially modified.
	Invocation Invocation `yaml:"invocation,omitempty"`
}

// template
type template struct {
	name       string
	runtime    string
	repository string
	fs         Filesystem
	manifest   templateConfig
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
			f.Builder = t.manifest.Builders["default"]
		}
	}
	if len(f.Builders) == 0 {
		f.Builders = t.manifest.Builders
	}
	if len(f.Buildpacks) == 0 {
		f.Buildpacks = t.manifest.Buildpacks
	}
	if len(f.BuildEnvs) == 0 {
		f.BuildEnvs = t.manifest.BuildEnvs
	}
	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = t.manifest.HealthEndpoints.Liveness
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = t.manifest.HealthEndpoints.Readiness
	}
	if f.Invocation.Format == "" {
		f.Invocation.Format = t.manifest.Invocation.Format
	}

	isManifest := func(p string) bool {
		_, f := path.Split(p)
		return f == templateManifest
	}

	return copyFromFS(".", f.Root, maskingFS{fs: t.fs, masked: isManifest}) // copy everything but manifest.yaml
}
