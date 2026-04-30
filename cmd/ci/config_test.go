package ci

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestResolveBuilder covers the branching logic that selects the correct
// build strategy for each runtime × local/remote combination.
func TestResolveBuilder(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		remote  bool
		want    string
		wantErr bool
	}{
		{name: "go local", runtime: "go", remote: false, want: "host"},
		{name: "go remote", runtime: "go", remote: true, want: "pack"},
		{name: "node local", runtime: "node", remote: false, want: "pack"},
		{name: "node remote", runtime: "node", remote: true, want: "pack"},
		{name: "typescript local", runtime: "typescript", remote: false, want: "pack"},
		{name: "typescript remote", runtime: "typescript", remote: true, want: "pack"},
		{name: "rust local", runtime: "rust", remote: false, want: "pack"},
		{name: "rust remote", runtime: "rust", remote: true, want: "pack"},
		{name: "quarkus local", runtime: "quarkus", remote: false, want: "pack"},
		{name: "quarkus remote", runtime: "quarkus", remote: true, want: "pack"},
		{name: "springboot local", runtime: "springboot", remote: false, want: "pack"},
		{name: "springboot remote", runtime: "springboot", remote: true, want: "pack"},
		{name: "python local", runtime: "python", remote: false, want: "host"},
		{name: "python remote", runtime: "python", remote: true, want: "s2i"},
		{name: "unknown runtime", runtime: "fortran", remote: false, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveBuilder(tc.runtime, tc.remote)
			if tc.wantErr {
				assert.Assert(t, err != nil, "expected an error for runtime %q", tc.runtime)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}
