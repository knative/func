package platform

import (
	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/files"
)

// Fulfills the prophecy set forth in https://github.com/buildpacks/rfcs/blob/b8abe33f2bdc58792acf0bd094dc4ce3c8a54dbb/text/0096-remove-stacks-mixins.md?plain=1#L97
// by returning an array of "VARIABLE=value" strings suitable for inclusion in your environment or complete breakfast.
func EnvVarsFor(tm files.TargetMetadata) []string {
	ret := []string{"CNB_TARGET_OS=" + tm.OS, "CNB_TARGET_ARCH=" + tm.Arch}
	ret = append(ret, "CNB_TARGET_ARCH_VARIANT="+tm.ArchVariant)
	var distName, distVersion string
	if tm.Distribution != nil {
		distName = tm.Distribution.Name
		distVersion = tm.Distribution.Version
	}
	ret = append(ret, "CNB_TARGET_DISTRO_NAME="+distName)
	ret = append(ret, "CNB_TARGET_DISTRO_VERSION="+distVersion)
	if tm.ID != "" {
		ret = append(ret, "CNB_TARGET_ID="+tm.ID)
	}
	return ret
}

func GetTargetMetadata(fromImage imgutil.Image) (*files.TargetMetadata, error) {
	tm := files.TargetMetadata{}
	var err error
	tm.OS, err = fromImage.OS()
	if err != nil {
		return &tm, err
	}
	tm.Arch, err = fromImage.Architecture()
	if err != nil {
		return &tm, err
	}
	tm.ArchVariant, err = fromImage.Variant()
	if err != nil {
		return &tm, err
	}
	labels, err := fromImage.Labels()
	if err != nil {
		return &tm, err
	}
	distName, distNameExists := labels[OSDistributionNameLabel]
	distVersion, distVersionExists := labels[OSDistributionVersionLabel]
	if distNameExists || distVersionExists {
		tm.Distribution = &files.OSDistribution{Name: distName, Version: distVersion}
	}
	if id, exists := labels[TargetLabel]; exists {
		tm.ID = id
	}

	return &tm, nil
}

// GetTargetOSFromFileSystem populates the target metadata you pass in if the information is available
// returns a boolean indicating whether it populated any data.
func GetTargetOSFromFileSystem(d fsutil.Detector, tm *files.TargetMetadata, logger log.Logger) {
	if d.HasSystemdFile() {
		contents, err := d.ReadSystemdFile()
		if err != nil {
			logger.Warnf("Encountered error trying to read /etc/os-release file: %s", err.Error())
			return
		}
		info := d.GetInfo(contents)
		if info.Version != "" || info.Name != "" {
			tm.OS = "linux"
			tm.Distribution = &files.OSDistribution{Name: info.Name, Version: info.Version}
		}
	}
}

// TargetSatisfiedForBuild treats optional fields (ArchVariant and Distributions) as wildcards if empty, returns true if all populated fields match
func TargetSatisfiedForBuild(t files.TargetMetadata, o buildpack.TargetMetadata) bool {
	if (o.Arch != "*" && t.Arch != o.Arch) || (o.OS != "*" && t.OS != o.OS) {
		return false
	}
	if t.ArchVariant != "" && o.ArchVariant != "" && t.ArchVariant != o.ArchVariant {
		return false
	}

	// if either of the lengths of Distributions are zero, treat it as a wildcard.
	if t.Distribution != nil && len(o.Distributions) > 0 {
		// this could be more efficient but the lists are probably short...
		found := false
		for _, odist := range o.Distributions {
			if t.Distribution.Name == odist.Name && t.Distribution.Version == odist.Version {
				found = true
				continue
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TargetSatisfiedForRebase treats optional fields (ArchVariant and Distribution fields) as wildcards if empty, returns true if all populated fields match
func TargetSatisfiedForRebase(t files.TargetMetadata, appTargetMetadata files.TargetMetadata) bool {
	if t.Arch != appTargetMetadata.Arch || t.OS != appTargetMetadata.OS {
		return false
	}
	if t.ArchVariant != "" && appTargetMetadata.ArchVariant != "" && t.ArchVariant != appTargetMetadata.ArchVariant {
		return false
	}

	if t.Distribution != nil && appTargetMetadata.Distribution != nil {
		if t.Distribution.Name != appTargetMetadata.Distribution.Name {
			return false
		}
		if t.Distribution.Version != appTargetMetadata.Distribution.Version {
			return false
		}
	}
	return true
}
