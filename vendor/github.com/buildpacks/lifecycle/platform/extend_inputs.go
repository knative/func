package platform

import (
	"os"
	"time"
)

// ExtendInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type ExtendInputs struct {
	AnalyzedPath   string
	AppDir         string
	BuildpacksDir  string
	GeneratedDir   string
	GroupPath      string
	ImageRef       string
	LayersDir      string
	PlanPath       string
	PlatformDir    string
	UID, GID       int
	KanikoCacheTTL time.Duration
}

// ResolveExtend accepts a ExtendInputs and returns a new ExtendInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveExtend(inputs ExtendInputs) (ExtendInputs, error) {
	resolvedInputs := inputs

	var err error
	if inputs.KanikoCacheTTL == 0 {
		if envTTL := os.Getenv(EnvKanikoCacheTTL); envTTL == "" {
			resolvedInputs.KanikoCacheTTL = DefaultKanikoCacheTTL
		} else if resolvedInputs.KanikoCacheTTL, err = time.ParseDuration(envTTL); err != nil {
			return ExtendInputs{}, err
		}
	}

	r.fillExtendDefaultPaths(&resolvedInputs)

	if err = r.resolveExtendDirPaths(&resolvedInputs); err != nil {
		return ExtendInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillExtendDefaultPaths(inputs *ExtendInputs) {
	if inputs.AnalyzedPath == PlaceholderAnalyzedPath {
		inputs.AnalyzedPath = defaultPath(PlaceholderAnalyzedPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GeneratedDir == PlaceholderGeneratedDir {
		inputs.GeneratedDir = defaultPath(PlaceholderGeneratedDir, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.PlanPath == PlaceholderPlanPath {
		inputs.PlanPath = defaultPath(PlaceholderPlanPath, inputs.LayersDir, r.platformAPI)
	}
}

func (r *InputsResolver) resolveExtendDirPaths(inputs *ExtendInputs) error {
	var err error
	if inputs.AppDir, err = absoluteIfNotEmpty(inputs.AppDir); err != nil {
		return err
	}
	if inputs.BuildpacksDir, err = absoluteIfNotEmpty(inputs.BuildpacksDir); err != nil {
		return err
	}
	if inputs.GeneratedDir, err = absoluteIfNotEmpty(inputs.GeneratedDir); err != nil {
		return err
	}
	if inputs.LayersDir, err = absoluteIfNotEmpty(inputs.LayersDir); err != nil {
		return err
	}
	if inputs.PlatformDir, err = absoluteIfNotEmpty(inputs.PlatformDir); err != nil {
		return err
	}
	return nil
}
