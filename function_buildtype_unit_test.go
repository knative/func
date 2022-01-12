//go:build !integration
// +build !integration

package function

import (
	"testing"
)

func Test_validateBuild(t *testing.T) {
	tests := []struct {
		name                   string
		build                  string
		allowDisabledBuildType bool
		allowUnset             bool
		errs                   int
	}{
		{
			name:                   "valid build type - local",
			build:                  "local",
			allowDisabledBuildType: true,
			allowUnset:             false,
			errs:                   0,
		},
		{
			name:                   "valid build type - git",
			build:                  "git",
			allowDisabledBuildType: true,
			allowUnset:             false,
			errs:                   0,
		},
		{
			name:                   "valid build type - disabled",
			build:                  "disabled",
			allowDisabledBuildType: true,
			allowUnset:             false,
			errs:                   0,
		},
		{
			name:                   "build type \"disabled\" is not allowed in this case",
			build:                  "disabled",
			allowDisabledBuildType: false,
			allowUnset:             false,
			errs:                   1,
		},
		{
			name:                   "invalid build type",
			build:                  "foo",
			allowDisabledBuildType: true,
			allowUnset:             false,
			errs:                   1,
		},
		{
			name:                   "build type not specified - valid option",
			allowDisabledBuildType: false,
			allowUnset:             true,
			errs:                   0,
		},
		{
			name:                   "build type not specified - invalid option",
			allowDisabledBuildType: false,
			allowUnset:             false,
			errs:                   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateBuildType(tt.build, tt.allowUnset, tt.allowDisabledBuildType); len(got) != tt.errs {
				t.Errorf("validateBuildType() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
