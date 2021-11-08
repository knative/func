package function

import (
	"context"
	"path/filepath"
)

type ITemplate interface {
	Name() string
	Runtime() string
	Write(ctx context.Context, name, destDir string) error
}

type staticTemplateConfig struct {
	// Runtime for which this template applies.
	Runtime string
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`
	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
}

// Template static template
type Template struct {
	name   string
	config staticTemplateConfig
	fs     Filesystem
	path   string
}

func (t Template) Name() string {
	return t.name
}

func (t Template) Runtime() string {
	return t.config.Runtime
}

func (t Template) Write(ctx context.Context, name, destDir string) error {
	// Validate paths:  (repo/)[templates/]<runtime>/<template>
	templatesPath := t.path
	if _, err := t.fs.Stat(templatesPath); err != nil {
		return ErrTemplatesNotFound
	}
	runtimePath := filepath.Join(templatesPath, t.Runtime())
	if _, err := t.fs.Stat(runtimePath); err != nil {
		return ErrRuntimeNotFound
	}
	templatePath := filepath.Join(runtimePath, t.Name())
	if _, err := t.fs.Stat(templatePath); err != nil {
		return ErrTemplateNotFound
	}

	// Copy the template files from the repo filesystem to the new Function's root
	err := copy(templatePath, destDir, t.fs)
	if err != nil {
		return err
	}

	var builder string
	if _, ok := t.config.BuildConfig.Builders["default"]; ok {
		builder = "default"
	} else {
		for k := range t.config.BuildConfig.Builders {
			builder = k
			break
		}
	}

	return Function{
		Name:            name,
		Root:            destDir,
		Runtime:         t.Runtime(),
		Template:        t.Name(),
		Builder:         builder,
		Builders:        t.config.BuildConfig.Builders,
		Buildpacks:      t.config.BuildConfig.Buildpacks,
		HealthEndpoints: t.config.HealthEndpoints,
	}.WriteConfig()
}
