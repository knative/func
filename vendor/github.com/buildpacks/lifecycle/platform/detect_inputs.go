package platform

import (
	"path/filepath"
)

// DetectInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type DetectInputs struct {
	AnalyzedPath  string
	AppDir        string
	BuildpacksDir string
	ExtensionsDir string
	GeneratedDir  string
	GroupPath     string
	LayersDir     string
	OrderPath     string
	PlanPath      string
	PlatformDir   string
}

// ResolveDetect accepts a DetectInputs and returns a new DetectInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveDetect(inputs DetectInputs) (DetectInputs, error) {
	resolvedInputs := inputs

	r.fillDetectDefaultPaths(&resolvedInputs)

	if err := r.resolveDetectDirPaths(&resolvedInputs); err != nil {
		return DetectInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillDetectDefaultPaths(inputs *DetectInputs) {
	if inputs.AnalyzedPath == PlaceholderAnalyzedPath {
		inputs.AnalyzedPath = defaultPath(PlaceholderAnalyzedPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GeneratedDir == PlaceholderGeneratedDir {
		inputs.GeneratedDir = defaultPath(PlaceholderGeneratedDir, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.OrderPath == PlaceholderOrderPath {
		inputs.OrderPath = defaultPath(PlaceholderOrderPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.PlanPath == PlaceholderPlanPath {
		inputs.PlanPath = defaultPath(PlaceholderPlanPath, inputs.LayersDir, r.platformAPI)
	}
}

func (r *InputsResolver) resolveDetectDirPaths(inputs *DetectInputs) error {
	var err error
	if inputs.AppDir, err = absoluteIfNotEmpty(inputs.AppDir); err != nil {
		return err
	}
	if inputs.BuildpacksDir, err = absoluteIfNotEmpty(inputs.BuildpacksDir); err != nil {
		return err
	}
	if inputs.ExtensionsDir, err = absoluteIfNotEmpty(inputs.ExtensionsDir); err != nil {
		return err
	}
	if inputs.LayersDir, err = absoluteIfNotEmpty(inputs.LayersDir); err != nil {
		return err
	}
	if inputs.GeneratedDir, err = absoluteIfNotEmpty(inputs.GeneratedDir); err != nil {
		return err
	}
	if inputs.PlatformDir, err = absoluteIfNotEmpty(inputs.PlatformDir); err != nil {
		return err
	}
	return nil
}

func absoluteIfNotEmpty(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return filepath.Abs(path)
}
