package platform

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
)

// DefaultCreateInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `create` phase.
func DefaultCreateInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.11"):
		inputs = defaultCreateInputs()
	case platformAPI.AtLeast("0.6"):
		inputs = defaultCreateInputs06To010()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultCreateInputs05()
	default:
		inputs = defaultCreateInputs03()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultCreateInputs() LifecycleInputs {
	ci := defaultCreateInputs06To010()
	ci.BuildConfigDir = envOrDefault(EnvBuildConfigDir, DefaultBuildConfigDir)
	ci.LauncherSBOMDir = DefaultBuildpacksioSBOMDir
	return ci
}

func defaultCreateInputs06To010() LifecycleInputs {
	ci := defaultCreateInputs05()
	ci.OrderPath = envOrDefault(EnvOrderPath, placeholderOrderPath)
	return ci
}

func defaultCreateInputs05() LifecycleInputs {
	ci := defaultCreateInputs03()
	ci.ProjectMetadataPath = envOrDefault(EnvProjectMetadataPath, placeholderProjectMetadataPath)
	ci.ReportPath = envOrDefault(EnvReportPath, placeholderReportPath)
	return ci
}

func defaultCreateInputs03() LifecycleInputs {
	return LifecycleInputs{
		AppDir:              envOrDefault(EnvAppDir, DefaultAppDir),                           // <app>
		BuildpacksDir:       envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir),             // <buildpacks> - FIXME: spec should be updated with this input
		CacheDir:            os.Getenv(EnvCacheDir),                                           // <cache-dir>
		CacheImageRef:       os.Getenv(EnvCacheImage),                                         // <cache-image>
		UseDaemon:           boolEnv(EnvUseDaemon),                                            // <daemon>
		GID:                 intEnv(EnvGID),                                                   // <gid>
		LaunchCacheDir:      os.Getenv(EnvLaunchCacheDir),                                     // <launch-cache>
		LauncherPath:        DefaultLauncherPath,                                              // <launcher>
		LayersDir:           envOrDefault(EnvLayersDir, DefaultLayersDir),                     // <layers>
		LogLevel:            envOrDefault(EnvLogLevel, DefaultLogLevel),                       // <log-level>
		OrderPath:           envOrDefault(EnvOrderPath, DefaultOrderPath),                     // <order>
		PlatformDir:         envOrDefault(EnvPlatformDir, DefaultPlatformDir),                 // <platform>
		PreviousImageRef:    os.Getenv(EnvPreviousImage),                                      // <previous-image>
		DefaultProcessType:  os.Getenv(EnvProcessType),                                        // <process-type>
		ProjectMetadataPath: envOrDefault(EnvProjectMetadataPath, DefaultProjectMetadataFile), // <project-metadata>
		ReportPath:          envOrDefault(EnvReportPath, DefaultReportFile),                   // <report> - not actually introduced until Platform API 0.4, but it is always written by the lifecycle
		RunImageRef:         os.Getenv(EnvRunImage),                                           // <run-image>
		SkipLayers:          boolEnv(EnvSkipRestore),                                          // <skip-restore>
		StackPath:           envOrDefault(EnvStackPath, DefaultStackPath),                     // <stack>
		AdditionalTags:      str.Slice{},                                                      // <tag>...
		UID:                 intEnv(EnvUID),                                                   // <uid>
		OutputImageRef:      "",                                                               // <image>
	}
}

func FillCreateImages(i *LifecycleInputs, logger log.Logger) error {
	if i.PreviousImageRef == "" {
		i.PreviousImageRef = i.OutputImageRef
	}
	switch {
	case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
		return errors.New(ErrSupplyOnlyOneRunImage)
	case i.DeprecatedRunImageRef != "":
		i.RunImageRef = i.DeprecatedRunImageRef
		return nil
	default:
		return fillRunImageFromStackTOMLIfNeeded(i, logger)
	}
}
