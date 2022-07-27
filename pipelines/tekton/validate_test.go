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
			name:     "Without runtime - without builder - without additional Buildpacks",
			function: fn.Function{},
			wantErr:  true,
		},
		{
			name:     "Without runtime - pack builder - without additional Buildpacks",
			function: fn.Function{},
			wantErr:  true,
		},
		{
			name:     "Without runtime - s2i builder",
			function: fn.Function{},
			wantErr:  true,
		},
		{
			name:     "Without runtime - without builder - with additional Buildpacks",
			function: fn.Function{Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Without runtime - pack builder - with additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Without runtime - s2i builder",
			function: fn.Function{Builder: fn.BuilderS2i, Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - without builder - without additional Buildpacks",
			function: fn.Function{Runtime: "node"},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - pack builder - without additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - s2i builder",
			function: fn.Function{Builder: fn.BuilderS2i, Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - pack builder - with additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Runtime: "node", Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - pack builder - without additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Runtime: "go"},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - pack builder - with additional Buildpacks",
			function: fn.Function{Runtime: "go", Buildpacks: testBuildpacks},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - s2i builder",
			function: fn.Function{Builder: fn.BuilderS2i, Runtime: "go"},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - Quarkus - pack builder - without additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - Quarkus - s2i builder",
			function: fn.Function{Builder: fn.BuilderS2i, Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Unsupported runtime - Rust - pack builder - without additional Buildpacks",
			function: fn.Function{Builder: fn.BuilderPack, Runtime: "rust"},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Rust - s2i builder",
			function: fn.Function{Builder: fn.BuilderS2i, Runtime: "rust"},
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
