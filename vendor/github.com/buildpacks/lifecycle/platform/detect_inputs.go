package platform

import (
	"github.com/buildpacks/lifecycle/api"
)

// DefaultDetectInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `detect` phase.
func DefaultDetectInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.11"):
		inputs = defaultDetectInputs()
	case platformAPI.AtLeast("0.10"):
		inputs = defaultDetectInputs010()
	case platformAPI.AtLeast("0.6"):
		inputs = defaultDetectInputs06To09()
	case platformAPI.AtLeast("0.5"):
		inputs = defaultDetectInputs05()
	default:
		inputs = defaultDetectInputs03To04()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultDetectInputs() LifecycleInputs {
	di := defaultDetectInputs010()
	di.BuildConfigDir = envOrDefault(EnvBuildConfigDir, DefaultBuildConfigDir)
	return di
}

func defaultDetectInputs010() LifecycleInputs {
	di := defaultDetectInputs06To09()
	di.AnalyzedPath = envOrDefault(EnvAnalyzedPath, placeholderAnalyzedPath)
	di.ExtensionsDir = envOrDefault(EnvExtensionsDir, DefaultExtensionsDir)
	di.GeneratedDir = envOrDefault(EnvGeneratedDir, placeholderGeneratedDir)
	return di
}

func defaultDetectInputs06To09() LifecycleInputs {
	di := defaultDetectInputs05()
	di.OrderPath = envOrDefault(EnvOrderPath, placeholderOrderPath)
	return di
}

func defaultDetectInputs05() LifecycleInputs {
	di := defaultDetectInputs03To04()
	di.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	di.PlanPath = envOrDefault(EnvPlanPath, placeholderPlanPath)
	return di
}

func defaultDetectInputs03To04() LifecycleInputs {
	return LifecycleInputs{
		AppDir:        envOrDefault(EnvAppDir, DefaultAppDir),               // <app>
		BuildpacksDir: envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir), // <buildpacks>
		GroupPath:     envOrDefault(EnvGroupPath, DefaultGroupFile),         // <group>
		LayersDir:     envOrDefault(EnvLayersDir, DefaultLayersDir),         // <layers>
		LogLevel:      envOrDefault(EnvLogLevel, DefaultLogLevel),           // <log-level>
		OrderPath:     envOrDefault(EnvOrderPath, DefaultOrderPath),         // <order>
		PlanPath:      envOrDefault(EnvPlanPath, DefaultPlanFile),           // <plan>
		PlatformDir:   envOrDefault(EnvPlatformDir, DefaultPlatformDir),     // <platform>
	}
}
