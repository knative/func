package tekton

import (
	"path/filepath"
	"testing"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

const (
	// TestRegistry for calculating destination image during tests.
	// Will be optional once we support in-cluster container registries
	// by default.  See TestRegistryRequired for details.
	TestRegistry = "example.com/alice"
)

func Test_createPipelineTemplate(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		builder string
		wantErr bool
	}{
		{
			name:    "correct - pack builder",
			root:    "testdata/testCreatePipelineTemplatePack",
			builder: builders.Pack,
			wantErr: false,
		},
		{
			name:    "correct - s2i builder",
			root:    "testdata/testCreatePipelineTemplateS2I",
			builder: builders.S2I,
			wantErr: false,
		},
		{
			name:    "incorrect - foo builder",
			root:    "testdata/testCreatePipelineTemplateFoo",
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

			err = createPipelineTemplate(f)

			if (err != nil) != tt.wantErr {
				t.Errorf("createPipelineTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fp := filepath.Join(root, resourcesDirectory, pipelineFileName)
			exists, err := FileExists(t, fp)
			if err != nil {
				t.Fatal(err)
			}

			if !exists != tt.wantErr {
				t.Errorf("a pipeline should be generated in %s", fp)
				return
			}
		})
	}
}

func Test_createPipelineRunTemplate(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		builder string
		wantErr bool
	}{
		{
			name:    "correct - pack builder",
			root:    "testdata/testCreatePipelineRunTemplatePack",
			builder: builders.Pack,
			wantErr: false,
		},
		{
			name:    "correct - s2i builder",
			root:    "testdata/testCreatePipelineRunTemplateS2I",
			builder: builders.S2I,
			wantErr: false,
		},
		{
			name:    "incorrect - foo builder",
			root:    "testdata/testCreatePipelineRunTemplateFoo",
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

			err = createPipelineRunTemplate(f)

			if (err != nil) != tt.wantErr {
				t.Errorf("createPipelineRunTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fp := filepath.Join(root, resourcesDirectory, pipelineRunFilenane)
			exists, err := FileExists(t, fp)
			if err != nil {
				t.Fatal(err)
			}

			if !exists != tt.wantErr {
				t.Errorf("a pipeline run should be generated in %s", fp)
				return
			}
		})
	}
}
