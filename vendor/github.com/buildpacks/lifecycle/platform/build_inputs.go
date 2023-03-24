package platform

import (
	"github.com/buildpacks/lifecycle/api"
)

// DefaultBuildInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `build` phase.
func DefaultBuildInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.11"):
		inputs = defaultBuildInputs()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultBuildInputs05To010()
	default:
		inputs = defaultBuildInputs03To04()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultBuildInputs() LifecycleInputs {
	bi := defaultBuildInputs05To010()
	bi.BuildConfigDir = envOrDefault(EnvBuildConfigDir, DefaultBuildConfigDir)
	return bi
}

func defaultBuildInputs05To010() LifecycleInputs {
	bi := defaultBuildInputs03To04()
	bi.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	bi.PlanPath = envOrDefault(EnvPlanPath, placeholderPlanPath)
	return bi
}

func defaultBuildInputs03To04() LifecycleInputs {
	return LifecycleInputs{
		AppDir:        envOrDefault(EnvAppDir, DefaultAppDir),               // <app>
		BuildpacksDir: envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir), // <buildpacks>
		GroupPath:     envOrDefault(EnvGroupPath, DefaultGroupFile),         // <group>
		LayersDir:     envOrDefault(EnvLayersDir, DefaultLayersDir),         // <layers>
		LogLevel:      envOrDefault(EnvLogLevel, DefaultLogLevel),           // <log-level>
		PlanPath:      envOrDefault(EnvPlanPath, DefaultPlanFile),           // <plan>
		PlatformDir:   envOrDefault(EnvPlatformDir, DefaultPlatformDir),     // <platform>
	}
}
