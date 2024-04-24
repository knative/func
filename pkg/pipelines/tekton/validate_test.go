package tekton

import (
	"testing"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
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
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack}},
			wantErr:  true,
		},
		{
			name:     "Without runtime - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I}},
			wantErr:  true,
		},
		{
			name:     "Without runtime - without builder - with additional Buildpacks",
			function: fn.Function{Build: fn.BuildSpec{Buildpacks: testBuildpacks}},
			wantErr:  true,
		},
		{
			name:     "Without runtime - pack builder - with additional Buildpacks",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack, Buildpacks: testBuildpacks}},
			wantErr:  true,
		},
		{
			name:     "Without runtime - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I, Buildpacks: testBuildpacks}},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - without builder - without additional Buildpacks",
			function: fn.Function{Runtime: "node"},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - pack builder - without additional Buildpacks",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack}, Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I}, Runtime: "node"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - pack builder - with additional Buildpacks",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack, Buildpacks: testBuildpacks}, Runtime: "node"},
			wantErr:  true,
		},
		{
			name:     "Supported runtime - Go - pack builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack}, Runtime: "go"},
			wantErr:  false,
		},
		{
			name:     "Unsupported runtime - Go - pack builder - with additional Buildpacks",
			function: fn.Function{Runtime: "go", Build: fn.BuildSpec{Buildpacks: testBuildpacks}},
			wantErr:  true,
		},
		{
			name:     "Unsupported runtime - Go - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I}, Runtime: "go"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - Quarkus - pack builder - without additional Buildpacks",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack}, Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - Quarkus - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I}, Runtime: "quarkus"},
			wantErr:  false,
		},
		{
			name:     "Supported runtime - Rust - pack builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.Pack}, Runtime: "rust"},
			wantErr:  false,
		},
		{
			name:     "Unsupported runtime - Rust - s2i builder",
			function: fn.Function{Build: fn.BuildSpec{Builder: builders.S2I}, Runtime: "rust"},
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
