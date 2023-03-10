package tekton

import (
	"context"
	"testing"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

func Test_createLocalResources(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		builder string
		wantErr bool
	}{
		{
			name:    "correct - pack builder",
			root:    "testdata/testCreateLocalResourcesPack",
			builder: builders.Pack,
			wantErr: false,
		},
		{
			name:    "correct - s2i builder",
			root:    "testdata/testCreateLocalResourcesS2I",
			builder: builders.S2I,
			wantErr: false,
		},
		{
			name:    "incorrect - foo builder",
			root:    "testdata/testCreateLocalResourcesFoo",
			builder: "foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root
			defer Using(t, root)()

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}

			f.Build.Builder = tt.builder
			f.Image = "docker.io/alice/" + f.Name
			f.Registry = TestRegistry

			pp := NewPipelinesProvider()
			err = pp.createLocalResources(context.Background(), f)
			if (err != nil) != tt.wantErr {
				t.Errorf("pp.createLocalResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
