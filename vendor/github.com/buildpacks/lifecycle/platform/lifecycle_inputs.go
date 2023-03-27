package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
)

// LifecycleInputs holds the values of command-line flags and args i.e., platform inputs to the lifecycle.
// Fields are the cumulative total of inputs across all lifecycle phases and all supported Platform APIs.
type LifecycleInputs struct {
	PlatformAPI           *api.Version
	AnalyzedPath          string
	AppDir                string
	BuildConfigDir        string
	BuildImageRef         string
	BuildpacksDir         string
	CacheDir              string
	CacheImageRef         string
	DefaultProcessType    string
	DeprecatedRunImageRef string
	ExtensionsDir         string
	GeneratedDir          string
	GroupPath             string
	KanikoDir             string
	LaunchCacheDir        string
	LauncherPath          string
	LauncherSBOMDir       string
	LayersDir             string
	LogLevel              string
	OrderPath             string
	OutputImageRef        string
	PlanPath              string
	PlatformDir           string
	PreviousImageRef      string
	ProjectMetadataPath   string
	ReportPath            string
	RunImageRef           string
	StackPath             string
	UID                   int
	GID                   int
	SkipLayers            bool
	UseDaemon             bool
	AdditionalTags        str.Slice // str.Slice satisfies the `Value` interface required by the `flag` package
	KanikoCacheTTL        time.Duration
}

func (i *LifecycleInputs) DestinationImages() []string {
	var ret []string
	ret = appendOnce(ret, i.OutputImageRef)
	ret = appendOnce(ret, i.AdditionalTags...)
	return ret
}

func (i *LifecycleInputs) Images() []string {
	var ret []string
	ret = appendOnce(ret, i.DestinationImages()...)
	ret = appendOnce(ret, i.PreviousImageRef, i.BuildImageRef, i.RunImageRef, i.DeprecatedRunImageRef, i.CacheImageRef)
	return ret
}

func (i *LifecycleInputs) RegistryImages() []string {
	var ret []string
	ret = appendOnce(ret, i.CacheImageRef)
	if i.UseDaemon {
		return ret
	}
	ret = appendOnce(ret, i.Images()...)
	return ret
}

func appendOnce(list []string, els ...string) []string {
	for _, el := range els {
		if el == "" {
			continue
		}
		if notIn(list, el) {
			list = append(list, el)
		}
	}
	return list
}

func notIn(list []string, str string) bool {
	for _, el := range list {
		if el == str {
			return false
		}
	}
	return true
}

var (
	ErrOutputImageRequired           = "image argument is required"
	ErrRunImageRequiredWhenNoStackMD = "-run-image is required when there is no stack metadata available"
	ErrSupplyOnlyOneRunImage         = "supply only one of -run-image or (deprecated) -image"
	ErrRunImageUnsupported           = "-run-image is unsupported"
	ErrImageUnsupported              = "-image is unsupported"
	MsgIgnoringLaunchCache           = "Ignoring -launch-cache, only intended for use with -daemon"
)

func ResolveInputs(phase LifecyclePhase, i *LifecycleInputs, logger log.Logger) error {
	// order of operations is important
	ops := []LifecycleInputsOperation{UpdatePlaceholderPaths, ResolveAbsoluteDirPaths}
	switch phase {
	case Analyze:
		if i.PlatformAPI.LessThan("0.7") {
			ops = append(ops, CheckCache)
		}
		ops = append(ops,
			FillAnalyzeImages,
			ValidateOutputImageProvided,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Build:
		// nop
	case Create:
		ops = append(ops,
			FillCreateImages,
			ValidateOutputImageProvided,
			CheckCache,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Detect:
		// nop
	case Export:
		ops = append(ops,
			FillExportRunImage,
			ValidateOutputImageProvided,
			CheckCache,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Extend:
		// nop
	case Rebase:
		ops = append(ops,
			ValidateRebaseRunImage,
			ValidateOutputImageProvided,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Restore:
		ops = append(ops, CheckCache)
	}

	var err error
	for _, op := range ops {
		if err = op(i, logger); err != nil {
			return err
		}
	}
	return nil
}

// operations

type LifecycleInputsOperation func(i *LifecycleInputs, logger log.Logger) error

func CheckCache(i *LifecycleInputs, logger log.Logger) error {
	if i.CacheImageRef == "" && i.CacheDir == "" {
		logger.Warn("No cached data will be used, no cache specified.")
	}
	return nil
}

func CheckLaunchCache(i *LifecycleInputs, logger log.Logger) error {
	if !i.UseDaemon && i.LaunchCacheDir != "" {
		logger.Warn(MsgIgnoringLaunchCache)
	}
	return nil
}

// fillRunImageFromStackTOMLIfNeeded updates the provided lifecycle inputs to include the run image from stack.toml if it is missing.
// When there are multiple run images in stack.toml, the run image with registry matching the output image is selected.
func fillRunImageFromStackTOMLIfNeeded(i *LifecycleInputs, logger log.Logger) error {
	if i.RunImageRef != "" {
		return nil
	}
	targetRegistry, err := parseRegistry(i.OutputImageRef)
	if err != nil {
		return err
	}
	stackMD, err := ReadStack(i.StackPath, logger)
	if err != nil {
		return err
	}
	i.RunImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New(ErrRunImageRequiredWhenNoStackMD)
	}
	return nil
}

func parseRegistry(providedRef string) (string, error) {
	ref, err := name.ParseReference(providedRef, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func ResolveAbsoluteDirPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.directoryPaths()
	for _, dir := range toUpdate {
		if *dir == "" {
			continue
		}
		abs, err := filepath.Abs(*dir)
		if err != nil {
			return err
		}
		*dir = abs
	}
	return nil
}

func (i *LifecycleInputs) directoryPaths() []*string {
	return []*string{
		&i.AppDir,
		&i.BuildConfigDir,
		&i.BuildpacksDir,
		&i.CacheDir,
		&i.ExtensionsDir,
		&i.GeneratedDir,
		&i.KanikoDir,
		&i.LaunchCacheDir,
		&i.LayersDir,
		&i.PlatformDir,
	}
}

const placeholderLayersDir = "<layers>"

var (
	placeholderAnalyzedPath        = filepath.Join(placeholderLayersDir, DefaultAnalyzedFile)
	placeholderGeneratedDir        = filepath.Join(placeholderLayersDir, DefaultGeneratedDir)
	placeholderGroupPath           = filepath.Join(placeholderLayersDir, DefaultGroupFile)
	placeholderOrderPath           = filepath.Join(placeholderLayersDir, DefaultOrderFile)
	placeholderPlanPath            = filepath.Join(placeholderLayersDir, DefaultPlanFile)
	placeholderProjectMetadataPath = filepath.Join(placeholderLayersDir, DefaultProjectMetadataFile)
	placeholderReportPath          = filepath.Join(placeholderLayersDir, DefaultReportFile)
)

func UpdatePlaceholderPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.placeholderPaths()
	for _, pp := range toUpdate {
		switch {
		case *pp == "":
			continue
		case *pp == placeholderOrderPath:
			*pp = i.defaultOrderPath()
		case strings.Contains(*pp, placeholderLayersDir):
			filename := filepath.Base(*pp)
			*pp = filepath.Join(i.configDir(), filename)
		default:
			// nop
		}
	}
	return nil
}

func (i *LifecycleInputs) defaultOrderPath() string {
	if i.PlatformAPI.LessThan("0.6") {
		return DefaultOrderPath
	}
	layersOrderPath := filepath.Join(i.LayersDir, "order.toml")
	if _, err := os.Stat(layersOrderPath); err != nil {
		return DefaultOrderPath
	}
	return layersOrderPath
}

func (i *LifecycleInputs) configDir() string {
	if i.PlatformAPI.LessThan("0.5") ||
		(i.LayersDir == "") { // i.LayersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return "." // the current working directory
	}
	return i.LayersDir
}

func (i *LifecycleInputs) placeholderPaths() []*string {
	return []*string{
		&i.AnalyzedPath,
		&i.GeneratedDir,
		&i.GroupPath,
		&i.OrderPath,
		&i.PlanPath,
		&i.ProjectMetadataPath,
		&i.ReportPath,
	}
}

// ValidateImageRefs ensures all provided image references are valid.
func ValidateImageRefs(i *LifecycleInputs, _ log.Logger) error {
	for _, imageRef := range i.Images() {
		_, err := name.ParseReference(imageRef, name.WeakValidation)
		if err != nil {
			return err
		}
	}
	return nil
}

func ValidateOutputImageProvided(i *LifecycleInputs, logger log.Logger) error {
	if i.OutputImageRef == "" {
		return errors.New(ErrOutputImageRequired)
	}
	return nil
}

// ValidateTargetsAreSameRegistry ensures all output images are on the same registry.
func ValidateTargetsAreSameRegistry(i *LifecycleInputs, _ log.Logger) error {
	if i.UseDaemon {
		return nil
	}
	return ValidateSameRegistry(i.DestinationImages()...)
}

func ValidateSameRegistry(tags ...string) error {
	var (
		reg        string
		registries = map[string]struct{}{}
	)
	for _, imageRef := range tags {
		ref, err := name.ParseReference(imageRef, name.WeakValidation)
		if err != nil {
			return err
		}
		reg = ref.Context().RegistryStr()
		registries[reg] = struct{}{}
	}

	if len(registries) > 1 {
		return errors.New("writing to multiple registries is unsupported")
	}
	return nil
}

// shared helpers

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func envOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}

func intEnv(k string) int {
	v := os.Getenv(k)
	d, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return d
}
