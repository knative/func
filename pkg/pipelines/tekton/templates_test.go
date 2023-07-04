package tekton

import (
	"path/filepath"
	"testing"

	"github.com/manifestival/manifestival"
	"github.com/manifestival/manifestival/fake"

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

func Test_createPipelineTemplatePAC(t *testing.T) {
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

			err = createPipelineTemplatePAC(f, make(map[string]string))

			if (err != nil) != tt.wantErr {
				t.Errorf("createPipelineTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fp := filepath.Join(root, resourcesDirectory, pipelineFileNamePAC)
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

func Test_createPipelineRunTemplatePAC(t *testing.T) {
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

			err = createPipelineRunTemplatePAC(f, make(map[string]string))

			if (err != nil) != tt.wantErr {
				t.Errorf("createPipelineRunTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fp := filepath.Join(root, resourcesDirectory, pipelineRunFilenamePAC)
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

// testData are used by Test_createAndApplyPipelineTemplate() and Test_createAndApplyPipelineRunTemplate()
var testData = []struct {
	name      string
	root      string
	builder   string
	runtime   string
	namespace string
	labels    map[string]string
	wantErr   bool
}{
	{
		name:      "correct - pack & node",
		root:      "testdata/testCreatePipelinePackNode",
		runtime:   "node",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & quarkus",
		root:      "testdata/testCreatePipelinePackQuarkus",
		runtime:   "quarkus",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & go",
		root:      "testdata/testCreatePipelinePackGo",
		runtime:   "go",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & python",
		root:      "testdata/testCreatePipelinePackPython",
		runtime:   "python",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & typescript",
		root:      "testdata/testCreatePipelinePackTypescript",
		runtime:   "typescript",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & springboot",
		root:      "testdata/testCreatePipelinePackSpringboot",
		runtime:   "springboot",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - pack & rust",
		root:      "testdata/testCreatePipelinePackRust",
		runtime:   "rust",
		builder:   builders.Pack,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & node",
		root:      "testdata/testCreatePipelineS2INode",
		runtime:   "node",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & quarkus",
		root:      "testdata/testCreatePipelineS2IQuarkus",
		runtime:   "quarkus",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & go",
		root:      "testdata/testCreatePipelineS2IGo",
		runtime:   "go",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & python",
		root:      "testdata/testCreatePipelineS2IPython",
		runtime:   "python",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & typescript",
		root:      "testdata/testCreatePipelineS2ITypescript",
		runtime:   "typescript",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & springboot",
		root:      "testdata/testCreatePipelineS2ISpringboot",
		runtime:   "springboot",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
	{
		name:      "correct - s2i & rust",
		root:      "testdata/testCreatePipelineS2IRust",
		runtime:   "rust",
		builder:   builders.S2I,
		namespace: "test-ns",
		wantErr:   false,
	},
}

func Test_createAndApplyPipelineTemplate(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			// save current function and restore it at the end
			old := manifestivalClient
			defer func() { manifestivalClient = old }()

			manifestivalClient = func() (manifestival.Client, error) {
				return fake.New(), nil
			}

			root := tt.root
			defer Using(t, root)()

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}

			f.Build.Builder = tt.builder
			f.Runtime = tt.runtime
			f.Image = "docker.io/alice/" + f.Name
			f.Registry = TestRegistry

			if err := createAndApplyPipelineTemplate(f, tt.namespace, tt.labels); (err != nil) != tt.wantErr {
				t.Errorf("createAndApplyPipelineTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_createAndApplyPipelineRunTemplate(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			// save current function and restore it at the end
			old := manifestivalClient
			defer func() { manifestivalClient = old }()

			manifestivalClient = func() (manifestival.Client, error) {
				return fake.New(), nil
			}

			root := tt.root + "Run"
			defer Using(t, root)()

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}

			f.Build.Builder = tt.builder
			f.Runtime = tt.runtime
			f.Image = "docker.io/alice/" + f.Name
			f.Registry = TestRegistry

			if err := createAndApplyPipelineRunTemplate(f, tt.namespace, tt.labels); (err != nil) != tt.wantErr {
				t.Errorf("createAndApplyPipelineRunTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
