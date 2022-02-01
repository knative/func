package function

import (
	"fmt"
)

const (
	BuildTypeDisabled = "disabled" // in general, type "disabled" should be allowed only for "func deploy comamnd" related functionality
	BuildTypeLocal    = "local"
	BuildTypeGit      = "git"
	//BuildTypeRemote   = "remote"	// TODO not supported yet
)

func AllBuildTypes() []string {
	return []string{BuildTypeLocal, BuildTypeGit, BuildTypeDisabled}
}

// ValidateBuild validates input Build type option from Function config.
// If "allowUnset" is set to true, the specified type could be "" -> fallback to DefaultBuildType,
// this option should be used for validating func.yaml file, where users don't have to specify the build type.
// Type "disabled" is allowed only if parameter "allowDisabledBuildType" is set to true,
// in general type "disabled" should be allowed only for "func deploy command" related functionality.
func ValidateBuildType(build string, allowUnset, allowDisabledBuildType bool) (errors []string) {
	valid := false

	switch build {
	case BuildTypeLocal, BuildTypeGit:
		valid = true
	case BuildTypeDisabled:
		if allowDisabledBuildType {
			valid = true
		}
	case "":
		if allowUnset {
			valid = true
		}
	}

	if !valid {
		return []string{fmt.Sprintf("specified build type \"%s\" is not valid, allowed build types are %s", build, SupportedBuildTypes(allowDisabledBuildType))}
	}
	return
}

// SupportedBuildTypes prints string with supported build types, type "disabled" is
// returned only if parameter "allowDisabledBuildType" is set to true
// in general, type "disabled" should be allowed only for "deploy" related functionality
func SupportedBuildTypes(allowDisabledBuildType bool) string {
	msg := ""
	if allowDisabledBuildType {
		msg = fmt.Sprintf("\"%s\", ", BuildTypeDisabled)
	}

	return fmt.Sprintf("%s\"%s\" or \"%s\"", msg, BuildTypeLocal, BuildTypeGit)
}
