package platform

import (
	"os"
	"path/filepath"
	"time"

	"github.com/buildpacks/lifecycle/api"
)

const (
	EnvAnalyzedPath        = "CNB_ANALYZED_PATH"
	EnvAppDir              = "CNB_APP_DIR"
	EnvBuildImage          = "CNB_BUILD_IMAGE"
	EnvBuildpacksDir       = "CNB_BUILDPACKS_DIR"
	EnvCacheDir            = "CNB_CACHE_DIR"
	EnvCacheImage          = "CNB_CACHE_IMAGE"
	EnvDeprecationMode     = "CNB_DEPRECATION_MODE"
	EnvExperimentalMode    = "CNB_EXPERIMENTAL_MODE"
	EnvExtensionsDir       = "CNB_EXTENSIONS_DIR"
	EnvGID                 = "CNB_GROUP_ID"
	EnvGeneratedDir        = "CNB_GENERATED_DIR"
	EnvGroupPath           = "CNB_GROUP_PATH"
	EnvKanikoCacheTTL      = "CNB_KANIKO_CACHE_TTL"
	EnvLaunchCacheDir      = "CNB_LAUNCH_CACHE_DIR"
	EnvLayersDir           = "CNB_LAYERS_DIR"
	EnvLogLevel            = "CNB_LOG_LEVEL"
	EnvNoColor             = "CNB_NO_COLOR" // defaults to false
	EnvOrderPath           = "CNB_ORDER_PATH"
	EnvPlanPath            = "CNB_PLAN_PATH"
	EnvPlatformAPI         = "CNB_PLATFORM_API"
	EnvPlatformDir         = "CNB_PLATFORM_DIR"
	EnvPreviousImage       = "CNB_PREVIOUS_IMAGE"
	EnvProcessType         = "CNB_PROCESS_TYPE"
	EnvProjectMetadataPath = "CNB_PROJECT_METADATA_PATH"
	EnvReportPath          = "CNB_REPORT_PATH"
	EnvRunImage            = "CNB_RUN_IMAGE"
	EnvSkipLayers          = "CNB_ANALYZE_SKIP_LAYERS" // defaults to false
	EnvSkipRestore         = "CNB_SKIP_RESTORE"        // defaults to false
	EnvStackPath           = "CNB_STACK_PATH"
	EnvUID                 = "CNB_USER_ID"
	EnvUseDaemon           = "CNB_USE_DAEMON" // defaults to false

	DefaultExperimentalMode = ModeError
	DefaultLogLevel         = "info"
	DefaultPlatformAPI      = "0.3"

	DefaultAnalyzedFile        = "analyzed.toml"
	DefaultGroupFile           = "group.toml"
	DefaultOrderFile           = "order.toml"
	DefaultPlanFile            = "plan.toml"
	DefaultProjectMetadataFile = "project-metadata.toml"
	DefaultReportFile          = "report.toml"

	ModeQuiet = "quiet"
	ModeWarn  = "warn"
	ModeError = "error"
)

var (
	DefaultAppDir         = filepath.Join(rootDir, "workspace")
	DefaultBuildpacksDir  = filepath.Join(rootDir, "cnb", "buildpacks")
	DefaultExtensionsDir  = filepath.Join(rootDir, "cnb", "extensions")
	DefaultKanikoCacheTTL = 14 * (24 * time.Hour)
	DefaultLauncherPath   = filepath.Join(rootDir, "cnb", "lifecycle", "launcher"+execExt)
	DefaultLayersDir      = filepath.Join(rootDir, "layers")
	DefaultOutputDir      = filepath.Join(rootDir, "layers")
	DefaultPlatformDir    = filepath.Join(rootDir, "platform")
	DefaultStackPath      = filepath.Join(rootDir, "cnb", "stack.toml")

	PlaceholderAnalyzedPath        = filepath.Join("<layers>", DefaultAnalyzedFile)
	PlaceholderGeneratedDir        = filepath.Join("<layers>", "generated")
	PlaceholderGroupPath           = filepath.Join("<layers>", DefaultGroupFile)
	PlaceholderOrderPath           = filepath.Join("<layers>", DefaultOrderFile)
	PlaceholderPlanPath            = filepath.Join("<layers>", DefaultPlanFile)
	PlaceholderProjectMetadataPath = filepath.Join("<layers>", DefaultProjectMetadataFile)
	PlaceholderReportPath          = filepath.Join("<layers>", DefaultReportFile)
)

func defaultPath(placeholderPath, layersDir string, platformAPI *api.Version) string {
	if placeholderPath == PlaceholderOrderPath {
		return defaultOrderPath(layersDir, platformAPI)
	}

	basename := filepath.Base(placeholderPath)
	if (platformAPI).LessThan("0.5") || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", basename)
	}
	return filepath.Join(layersDir, basename)
}

func defaultOrderPath(layersDir string, platformAPI *api.Version) string {
	cnbOrderPath := filepath.Join(rootDir, "cnb", "order.toml")
	if platformAPI.LessThan("0.6") {
		return cnbOrderPath
	}

	layersOrderPath := filepath.Join(layersDir, "order.toml")
	if _, err := os.Stat(layersOrderPath); err != nil {
		return cnbOrderPath
	}
	return layersOrderPath
}
