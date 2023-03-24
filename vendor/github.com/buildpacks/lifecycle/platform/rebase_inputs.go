package platform

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
)

// DefaultRebaseInputs accepts a Platform API version and returns a set of lifecycle inputs
// with default values filled in for the `rebase` phase.
func DefaultRebaseInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.5"):
		inputs = defaultRebaseInputs()
	default:
		inputs = defaultRebaseInputs03()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultRebaseInputs() LifecycleInputs {
	ri := defaultRebaseInputs03()
	ri.ReportPath = envOrDefault(EnvReportPath, placeholderReportPath)
	return ri
}

func defaultRebaseInputs03() LifecycleInputs {
	return LifecycleInputs{
		UseDaemon:   boolEnv(EnvUseDaemon),                          // <daemon>
		GID:         intEnv(EnvGID),                                 // <gid>
		LogLevel:    envOrDefault(EnvLogLevel, DefaultLogLevel),     // <log-level>
		ReportPath:  envOrDefault(EnvReportPath, DefaultReportFile), // <report> - not actually introduced until Platform API 0.4, but it is always written by the lifecycle
		RunImageRef: os.Getenv(EnvRunImage),                         // <run-image>
		UID:         intEnv(EnvUID),                                 // <uid>
	}
}

func ValidateRebaseRunImage(i *LifecycleInputs, _ log.Logger) error {
	switch {
	case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
		return errors.New(ErrSupplyOnlyOneRunImage)
	case i.DeprecatedRunImageRef != "":
		i.RunImageRef = i.DeprecatedRunImageRef
		return nil
	default:
		return nil
	}
}
