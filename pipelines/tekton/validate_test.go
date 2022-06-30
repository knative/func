//go:build !integration
// +build !integration

package tekton

import (
	"testing"

	fn "knative.dev/kn-plugin-func"
)

func Test_validatePipeline(t *testing.T) {

	testBuildpacks := []string{"quay.io/foo/my-buildpack"}

	tests := []struct {
		name     string
		function fn.Function
		wantErr  bool
	}{
		{
			name:     "Without runtime - without additional Buildpacks",
			function: fn.Function{},
			wantErr:  true,
		},
		{
			name:     "Without runtime - with additional Buildpacks",
			function: fn.Function{Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - without additional Buildpacks",
			function: fn.Function{Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - with additional Buildpacks",
			function: fn.Function{Runtime: "node", Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - without additional Buildpacks",
			function: fn.Function{Runtime: "go"},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - Quarkus - without additional Buildpacks",
			function: fn.Function{Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Unsupported runtime - Rust - without additional Buildpacks",
			function: fn.Function{Runtime: "rust"},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - with additional Buildpacks",
			function: fn.Function{Runtime: "go", Buildpacks: testBuildpacks},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipeline(tt.function)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePipeline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
