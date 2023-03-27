package platform

import (
	"os"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
)

// DefaultAnalyzeInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `analyze` phase.
func DefaultAnalyzeInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.9"):
		inputs = defaultAnalyzeInputs()
	case platformAPI.AtLeast("0.7"):
		inputs = defaultAnalyzeInputs07()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultAnalyzeInputs05To06()
	default:
		inputs = defaultAnalyzeInputs03To04()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultAnalyzeInputs() LifecycleInputs {
	ai := defaultAnalyzeInputs07()
	ai.LaunchCacheDir = os.Getenv(EnvLaunchCacheDir)
	return ai
}

func defaultAnalyzeInputs07() LifecycleInputs {
	ai := defaultAnalyzeInputs05To06()
	ai.AdditionalTags = str.Slice{}
	ai.CacheDir = "" // removed
	ai.PreviousImageRef = os.Getenv(EnvPreviousImage)
	ai.RunImageRef = os.Getenv(EnvRunImage)
	ai.StackPath = envOrDefault(EnvStackPath, DefaultStackPath)
	return ai
}

func defaultAnalyzeInputs05To06() LifecycleInputs {
	ai := defaultAnalyzeInputs03To04()
	ai.AnalyzedPath = envOrDefault(EnvAnalyzedPath, placeholderAnalyzedPath)
	ai.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	return ai
}

func defaultAnalyzeInputs03To04() LifecycleInputs {
	return LifecycleInputs{
		AnalyzedPath:   envOrDefault(EnvAnalyzedPath, DefaultAnalyzedFile), // <analyzed>
		CacheDir:       os.Getenv(EnvCacheDir),                             // <cache-dir>
		CacheImageRef:  os.Getenv(EnvCacheImage),                           // <cache-image>
		UseDaemon:      boolEnv(EnvUseDaemon),                              // <daemon>
		GID:            intEnv(EnvGID),                                     // <gid>
		GroupPath:      envOrDefault(EnvGroupPath, DefaultGroupFile),       // <group>
		OutputImageRef: "",                                                 // <image>
		LayersDir:      envOrDefault(EnvLayersDir, DefaultLayersDir),       // <layers>
		LogLevel:       envOrDefault(EnvLogLevel, DefaultLogLevel),         // <log-level>
		SkipLayers:     boolEnv(EnvSkipLayers),                             // <skip-layers>
		UID:            intEnv(EnvUID),                                     // <uid>
	}
}

func FillAnalyzeImages(i *LifecycleInputs, logger log.Logger) error {
	if i.PreviousImageRef == "" {
		i.PreviousImageRef = i.OutputImageRef
	}
	if i.PlatformAPI.LessThan("0.7") {
		return nil
	}
	return fillRunImageFromStackTOMLIfNeeded(i, logger)
}
