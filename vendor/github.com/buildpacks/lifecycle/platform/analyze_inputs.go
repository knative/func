package platform

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/str"
)

// AnalyzeInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type AnalyzeInputs struct {
	AdditionalTags   str.Slice // satisfies the `Value` interface required by the `flag` package
	AnalyzedPath     string
	CacheImageRef    string
	LaunchCacheDir   string
	LayersDir        string
	LegacyCacheDir   string
	LegacyGroupPath  string
	OutputImageRef   string
	PreviousImageRef string
	RunImageRef      string
	StackPath        string
	UID              int
	GID              int
	SkipLayers       bool
	UseDaemon        bool
}

// RegistryImages returns the inputs that are images in a registry.
func (a AnalyzeInputs) RegistryImages() []string {
	var images []string
	images = appendNotEmpty(images, a.CacheImageRef)
	if !a.UseDaemon {
		images = appendNotEmpty(images, a.PreviousImageRef, a.RunImageRef, a.OutputImageRef)
		images = appendNotEmpty(images, a.AdditionalTags...)
	}
	return images
}

// ResolveAnalyze accepts an AnalyzeInputs and returns a new AnalyzeInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveAnalyze(inputs AnalyzeInputs, logger Logger) (AnalyzeInputs, error) {
	resolvedInputs := inputs

	if err := r.fillDefaults(&resolvedInputs, logger); err != nil {
		return AnalyzeInputs{}, err
	}

	if err := r.validate(resolvedInputs, logger); err != nil {
		return AnalyzeInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillDefaults(inputs *AnalyzeInputs, logger Logger) error {
	if inputs.AnalyzedPath == PlaceholderAnalyzedPath {
		inputs.AnalyzedPath = defaultPath(PlaceholderAnalyzedPath, inputs.LayersDir, r.platformAPI)
	}

	if inputs.LegacyGroupPath == PlaceholderGroupPath {
		inputs.LegacyGroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}

	if inputs.PreviousImageRef == "" {
		inputs.PreviousImageRef = inputs.OutputImageRef
	}

	return r.fillRunImage(inputs, logger)
}

func (r *InputsResolver) fillRunImage(inputs *AnalyzeInputs, logger Logger) error {
	if r.platformAPI.LessThan("0.7") || inputs.RunImageRef != "" {
		return nil
	}

	targetRegistry, err := parseRegistry(inputs.OutputImageRef)
	if err != nil {
		return err
	}

	stackMD, err := readStack(inputs.StackPath, logger)
	if err != nil {
		return err
	}

	inputs.RunImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New("-run-image is required when there is no stack metadata available")
	}
	return nil
}

func (r *InputsResolver) validate(inputs AnalyzeInputs, logger Logger) error {
	if inputs.OutputImageRef == "" {
		return errors.New("image argument is required")
	}

	if !inputs.UseDaemon {
		if err := ensureSameRegistry(inputs.PreviousImageRef, inputs.OutputImageRef); err != nil {
			return errors.Wrap(err, "ensuring previous image and exported image are on same registry")
		}

		if inputs.LaunchCacheDir != "" {
			logger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		}
	}

	if err := image.ValidateDestinationTags(inputs.UseDaemon, append(inputs.AdditionalTags, inputs.OutputImageRef)...); err != nil {
		return errors.Wrap(err, "validating image tag(s)")
	}

	if r.platformAPI.AtLeast("0.7") {
		return nil
	}

	if inputs.CacheImageRef == "" && inputs.LegacyCacheDir == "" {
		logger.Warn("Not restoring cached layer metadata, no cache flag specified.")
	}
	return nil
}
