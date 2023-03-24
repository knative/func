package platform

import (
	"os"
	"time"

	"github.com/buildpacks/lifecycle/api"
)

// DefaultExtendInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `extend` phase.
func DefaultExtendInputs(platformAPI *api.Version) LifecycleInputs {
	return LifecycleInputs{
		AnalyzedPath:   envOrDefault(EnvAnalyzedPath, placeholderAnalyzedPath),     // <analyzed>
		AppDir:         envOrDefault(EnvAppDir, DefaultAppDir),                     // <app>
		BuildpacksDir:  envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir),       // <buildpacks>
		GeneratedDir:   envOrDefault(EnvGeneratedDir, placeholderGeneratedDir),     // <generated>
		GID:            intEnv(EnvGID),                                             // <gid>
		GroupPath:      envOrDefault(EnvGroupPath, placeholderGroupPath),           // <group>
		KanikoCacheTTL: timeEnvOrDefault(EnvKanikoCacheTTL, DefaultKanikoCacheTTL), // <kaniko-cache-ttl>
		LayersDir:      envOrDefault(EnvLayersDir, DefaultLayersDir),               // <layers>
		LogLevel:       envOrDefault(EnvLogLevel, DefaultLogLevel),                 // <log-level>
		PlanPath:       envOrDefault(EnvPlanPath, placeholderPlanPath),             // <plan>
		PlatformDir:    envOrDefault(EnvPlatformDir, DefaultPlatformDir),           // <platform>
		PlatformAPI:    platformAPI,
		UID:            intEnv(EnvUID), // <uid>
	}
}

func timeEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	envTTL := os.Getenv(key)
	if envTTL == "" {
		return defaultVal
	}
	ttl, err := time.ParseDuration(envTTL)
	if err != nil {
		return defaultVal
	}
	return ttl
}
