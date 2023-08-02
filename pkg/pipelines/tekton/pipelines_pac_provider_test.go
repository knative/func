package tekton

import (
	"context"
	"path/filepath"
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
			err = pp.createLocalPACResources(context.Background(), f)
			if (err != nil) != tt.wantErr {
				t.Errorf("pp.createLocalResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_deleteAllPipelineTemplates(t *testing.T) {
	root := "testdata/deleteAllPipelineTemplates"
	defer Using(t, root)()

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	f.Build.Builder = builders.Pack
	f.Build.Git.URL = "https://foo.bar/repo/function"
	f.Image = "docker.io/alice/" + f.Name
	f.Registry = TestRegistry

	pp := NewPipelinesProvider()
	err = pp.createLocalPACResources(context.Background(), f)
	if err != nil {
		t.Errorf("unexpected error while running pp.createLocalResources() error = %v", err)
	}

	errMsg := deleteAllPipelineTemplates(f)
	if errMsg != "" {
		t.Errorf("unexpected error while running deleteAllPipelineTemplates() error message = %s", errMsg)
	}

	fp := filepath.Join(root, resourcesDirectory)
	exists, err := FileExists(t, fp)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("directory with pipeline resources shouldn't exist on path = %s", fp)
	}
}
