//go:build !integration
// +build !integration

package function

import (
	"testing"
)

func Test_validateBuilder(t *testing.T) {
	tests := []struct {
		name       string
		builder    string
		allowUnset bool
		errs       int
	}{
		{
			name:       "valid builder - pack",
			builder:    "pack",
			allowUnset: false,
			errs:       0,
		},
		{
			name:       "valid builder - s2i",
			builder:    "s2i",
			allowUnset: false,
			errs:       0,
		},
		{
			name:       "invalid builder",
			builder:    "foo",
			allowUnset: false,
			errs:       1,
		},
		{
			name:       "builder not specified - valid option",
			allowUnset: true,
			errs:       0,
		},
		{
			name:       "builder not specified - invalid option",
			allowUnset: false,
			errs:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateBuilder(tt.builder, tt.allowUnset); len(got) != tt.errs {
				t.Errorf("ValidateBuilder() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
