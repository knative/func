package tekton

import (
	"bytes"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/manifestival/manifestival"
	"github.com/manifestival/manifestival/fake"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	. "knative.dev/func/pkg/testing"
)

const (
	// TestRegistry for calculating destination image during tests.
	// Will be optional once we support in-cluster container registries
	// by default.  See TestRegistryRequired for details.
	TestRegistry = "example.com/alice"
)

func Test_isInsecureRegistry(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		want     bool
	}{
		{"localhost without port", "localhost", true},
		{"127.0.0.1 without port", "127.0.0.1", true},
		{"cluster local registry without port", "registry.default.svc.cluster.local", true},
		{"localhost with port 5000", "localhost:5000", true},
		{"127.0.0.1 with port 5000", "127.0.0.1:5000", true},
		{"cluster local registry with port 5000", "registry.default.svc.cluster.local:5000", true},
		{"external registry", "docker.io", false},
		{"external registry with port", "quay.io:443", false},
		{"similar but not matching", "localhost.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInsecureRegistry(tt.registry); got != tt.want {
				t.Errorf("isInsecureRegistry(%q) = %v, want %v", tt.registry, got, tt.want)
			}
		})
	}
}

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

func Test_createAndApplyPipelineRunTemplate(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			// save current function and restore it at the end
			old := manifestivalClient
			defer func() { manifestivalClient = old }()

			manifestivalClient = func(_ *k8s.Client) (manifestival.Client, error) {
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

			if err := createAndApplyPipelineRunTemplate(nil, f, tt.namespace, tt.labels); (err != nil) != tt.wantErr {
				t.Errorf("createAndApplyPipelineRunTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// strictTektonDecoder returns a strict deserializer that rejects unknown fields
// in Tekton v1 resources (Pipeline, PipelineRun, etc.).
func strictTektonDecoder(t *testing.T) runtime.Decoder {
	t.Helper()
	myScheme := runtime.NewScheme()
	if err := tektonv1.AddToScheme(myScheme); err != nil {
		t.Fatal(err)
	}
	codecs := serializer.NewCodecFactory(myScheme, serializer.EnableStrict)
	return codecs.UniversalDeserializer()
}

// renderTemplate renders a Go text/template with the given data and returns the result.
func renderTemplate(t *testing.T, name, tmplStr string, data any) []byte {
	t.Helper()
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}
	return buf.Bytes()
}

// TestPipelineTemplatesValidate renders each Pipeline template with real
// inline task specs and decodes it into a typed tektonv1.Pipeline using a
// strict deserializer that rejects unknown fields.
func TestPipelineTemplatesValidate(t *testing.T) {
	buildpacksTaskRef, err := getTaskSpec(getBuildpackTask())
	if err != nil {
		t.Fatal(err)
	}
	s2iTaskRef, err := getTaskSpec(getS2ITask())
	if err != nil {
		t.Fatal(err)
	}

	data := templateData{
		FunctionName:          "myfunc",
		Annotations:           map[string]string{"test": "val"},
		Labels:                map[string]string{"test": "val"},
		PipelineName:          "myfunc-pipeline",
		TlsVerify:             "true",
		Registry:              "docker.io/alice",
		FuncBuildpacksTaskRef: buildpacksTaskRef,
		FuncS2iTaskRef:        s2iTaskRef,
	}

	tests := []struct {
		name    string
		tmplStr string
	}{
		{"packPipelineTemplate", packPipelineTemplate},
		{"s2iPipelineTemplate", s2iPipelineTemplate},
	}

	decode := strictTektonDecoder(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderTemplate(t, tt.name, tt.tmplStr, data)
			obj, _, err := decode.Decode(rendered, nil, nil)
			if err != nil {
				t.Fatalf("failed to decode Pipeline: %v", err)
			}
			if _, ok := obj.(*tektonv1.Pipeline); !ok {
				t.Fatalf("expected *Pipeline, got %T", obj)
			}
		})
	}
}

// TestPipelineRunTemplatesValidate renders each PipelineRun template with sample
// data and decodes it into a typed tektonv1.PipelineRun using a strict
// deserializer. This catches unknown/misplaced fields (e.g. spec.podTemplate
// instead of spec.taskRunTemplate.podTemplate) that string-based tests miss.
func TestPipelineRunTemplatesValidate(t *testing.T) {
	data := templateData{
		FunctionName:  "myfunc",
		Annotations:   map[string]string{"test": "val"},
		Labels:        map[string]string{"test": "val"},
		ContextDir:    ".",
		FunctionImage: "docker.io/alice/myfunc",
		Registry:      "docker.io/alice",
		BuilderImage:  "gcr.io/paketo-buildpacks/builder:base",
		BuildEnvs:     []string{"="},

		PipelineName:    "myfunc-pipeline",
		PipelineRunName: "myfunc-pipeline-run-",
		PvcName:         "myfunc-pvc",
		SecretName:      "myfunc-secret",

		PipelinesTargetBranch: "main",
		PipelineYamlURL:       ".tekton/pipeline-pac.yaml",
		S2iImageScriptsUrl:    "image:///usr/libexec/s2i",
		TlsVerify:             "true",
		RepoUrl:               "https://example.com/repo",
		Revision:              "main",
		Commit:                "abc123",
	}

	tests := []struct {
		name    string
		tmplStr string
	}{
		{"packRunTemplate", packRunTemplate},
		{"packRunTemplatePAC", packRunTemplatePAC},
		{"s2iRunTemplate", s2iRunTemplate},
		{"s2iRunTemplatePAC", s2iRunTemplatePAC},
	}

	decode := strictTektonDecoder(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderTemplate(t, tt.name, tt.tmplStr, data)
			obj, _, err := decode.Decode(rendered, nil, nil)
			if err != nil {
				t.Fatalf("failed to decode PipelineRun: %v", err)
			}
			if _, ok := obj.(*tektonv1.PipelineRun); !ok {
				t.Fatalf("expected *PipelineRun, got %T", obj)
			}
		})
	}
}
