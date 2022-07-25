//go:build !integration
// +build !integration

package function

import (
	"testing"
)

func Test_validateBuilder(t *testing.T) {
	tests := []struct {
		name      string
		builder   string
		wantError bool
	}{
		{
			name:      "valid builder - pack",
			builder:   "pack",
			wantError: false,
		},
		{
			name:      "valid builder - s2i",
			builder:   "s2i",
			wantError: false,
		},
		{
			name:      "invalid builder",
			builder:   "foo",
			wantError: true,
		},
		{
			name:      "builder not specified - invalid option",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuilder(tt.builder)
			if tt.wantError != (err != nil) {
				t.Errorf("ValidateBuilder() = Wanted error %v but actually got %v", tt.wantError, err)
			}
		})
	}
}
