package platform

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
)

// DefaultExportInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `export` phase.
func DefaultExportInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.11"):
		inputs = defaultExportInputs()
	case platformAPI.AtLeast("0.7"):
		inputs = defaultExportInputs07To010()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultExportInputs05To06()
	default:
		inputs = defaultExportInputs03()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultExportInputs() LifecycleInputs {
	ei := defaultExportInputs07To010()
	ei.LauncherSBOMDir = DefaultBuildpacksioSBOMDir
	return ei
}

func defaultExportInputs07To010() LifecycleInputs {
	ei := defaultExportInputs05To06()
	ei.RunImageRef = "" // removed
	return ei
}

func defaultExportInputs05To06() LifecycleInputs {
	ei := defaultExportInputs03()
	ei.AnalyzedPath = envOrDefault(EnvAnalyzedPath, placeholderAnalyzedPath)
	ei.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	ei.ProjectMetadataPath = envOrDefault(EnvProjectMetadataPath, placeholderProjectMetadataPath)
	ei.ReportPath = envOrDefault(EnvReportPath, placeholderReportPath)
	return ei
}

func defaultExportInputs03() LifecycleInputs {
	return LifecycleInputs{
		AdditionalTags:      str.Slice{},                                                      // [<image>...]
		AnalyzedPath:        envOrDefault(EnvAnalyzedPath, DefaultAnalyzedFile),               // <analyzed>
		AppDir:              envOrDefault(EnvAppDir, DefaultAppDir),                           // <app>
		CacheDir:            os.Getenv(EnvCacheDir),                                           // <cache-dir>
		CacheImageRef:       os.Getenv(EnvCacheImage),                                         // <cache-image>
		UseDaemon:           boolEnv(EnvUseDaemon),                                            // <daemon>
		GID:                 intEnv(EnvGID),                                                   // <gid>
		GroupPath:           envOrDefault(EnvGroupPath, DefaultGroupFile),                     // <group>
		OutputImageRef:      "",                                                               // <image>
		LaunchCacheDir:      os.Getenv(EnvLaunchCacheDir),                                     // <launch-cache>
		LauncherPath:        DefaultLauncherPath,                                              // <launcher>
		LayersDir:           envOrDefault(EnvLayersDir, DefaultLayersDir),                     // <layers>
		LogLevel:            envOrDefault(EnvLogLevel, DefaultLogLevel),                       // <log-level>
		DefaultProcessType:  os.Getenv(EnvProcessType),                                        // <process-type>
		ProjectMetadataPath: envOrDefault(EnvProjectMetadataPath, DefaultProjectMetadataFile), // <project-metadata>
		ReportPath:          envOrDefault(EnvReportPath, DefaultReportFile),                   // <report> - not actually introduced until Platform API 0.4, but it is always written by the lifecycle
		RunImageRef:         os.Getenv(EnvRunImage),                                           // <run-image>
		StackPath:           envOrDefault(EnvStackPath, DefaultStackPath),                     // <stack>
		UID:                 intEnv(EnvUID),                                                   // <uid>
	}
}

func FillExportRunImage(i *LifecycleInputs, logger log.Logger) error {
	supportsRunImageFlag := i.PlatformAPI.LessThan("0.7")
	if supportsRunImageFlag {
		switch {
		case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
			return errors.New(ErrSupplyOnlyOneRunImage)
		case i.RunImageRef != "":
			return nil
		case i.DeprecatedRunImageRef != "":
			i.RunImageRef = i.DeprecatedRunImageRef
			return nil
		default:
			return fillRunImageFromStackTOMLIfNeeded(i, logger)
		}
	} else {
		switch {
		case i.RunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
			return errors.New(ErrRunImageUnsupported)
		case i.DeprecatedRunImageRef != "":
			return errors.New(ErrImageUnsupported)
		default:
			analyzedMD, err := ReadAnalyzed(i.AnalyzedPath, logger)
			if err != nil {
				return err
			}
			if analyzedMD.RunImage == nil || analyzedMD.RunImage.Reference == "" {
				return errors.New("run image not found in analyzed metadata")
			}
			i.RunImageRef = analyzedMD.RunImage.Reference
			return nil
		}
	}
}
