package platform

import (
	"os"

	"github.com/buildpacks/lifecycle/api"
)

// DefaultRestoreInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `restore` phase.
func DefaultRestoreInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.10"):
		inputs = defaultRestoreInputs()
	case platformAPI.AtLeast("0.7"):
		inputs = defaultRestoreInputs07To09()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultRestoreInputs05To06()
	default:
		inputs = defaultRestoreInputs03To04()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultRestoreInputs() LifecycleInputs {
	ri := defaultRestoreInputs07To09()
	ri.BuildImageRef = os.Getenv(EnvBuildImage)
	ri.KanikoDir = "/kaniko"
	return ri
}

func defaultRestoreInputs07To09() LifecycleInputs {
	ri := defaultRestoreInputs05To06()
	ri.AnalyzedPath = envOrDefault(EnvAnalyzedPath, placeholderAnalyzedPath)
	ri.SkipLayers = false
	return ri
}

func defaultRestoreInputs05To06() LifecycleInputs {
	ri := defaultRestoreInputs03To04()
	ri.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	return ri
}

func defaultRestoreInputs03To04() LifecycleInputs {
	return LifecycleInputs{
		CacheDir:      os.Getenv(EnvCacheDir),                       // <cache-dir>
		CacheImageRef: os.Getenv(EnvCacheImage),                     // <cache-image>
		GID:           intEnv(EnvGID),                               // <gid>
		GroupPath:     envOrDefault(EnvGroupPath, DefaultGroupFile), // <group>
		LayersDir:     envOrDefault(EnvLayersDir, DefaultLayersDir), // <layers>
		LogLevel:      envOrDefault(EnvLogLevel, DefaultLogLevel),   // <log-level>
		UID:           intEnv(EnvUID),                               // <uid>
	}
}
