package tekton

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/manifestival/manifestival"
	"gopkg.in/yaml.v3"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	// Local resources properties
	resourcesDirectory     = ".tekton"
	pipelineFileName       = "pipeline.yaml"
	pipelineRunFilenane    = "pipeline-run.yaml"
	pipelineFileNamePAC    = "pipeline-pac.yaml"
	pipelineRunFilenamePAC = "pipeline-run-pac.yaml"

	// Tasks references for PAC PipelineRun that are defined in the annotations
	taskGitCloneRef = "git-clone"

	// Following part holds a reference to Git Clone Task to be used in Pipeline template,
	// the usage depends whether we use direct code upload or Git reference for a standard (non PAC) on-cluster build
	taskGitClonePACTaskRef = `- name: fetch-sources
      params:
        - name: url
          value: $(params.gitRepository)
        - name: revision
          value: $(params.gitRevision)
      taskRef:
        kind: Task
        name: git-clone
      workspaces:
        - name: output
          workspace: source-workspace`
	// TODO fix Tekton Hub reference
	taskGitCloneTaskRef = `- name: fetch-sources
      params:
        - name: url
          value: $(params.gitRepository)
        - name: revision
          value: $(params.gitRevision)
      taskRef:
        resolver: hub
        params:
          - name: kind
            value: task
          - name: name
            value: git-clone
          - name: version
            value: "0.4"
      workspaces:
        - name: output
          workspace: source-workspace`
	runAfterFetchSourcesRef = `runAfter:
        - fetch-sources`

	// S2I related properties
	defaultS2iImageScriptsUrl = "image:///usr/libexec/s2i"
	quarkusS2iImageScriptsUrl = "image:///usr/local/s2i"

	// The branch or tag we are targeting with Pipelines (ie: main, refs/tags/*)
	defaultPipelinesTargetBranch = "main"
)

type templateData struct {
	FunctionName  string
	Annotations   map[string]string
	Labels        map[string]string
	ContextDir    string
	FunctionImage string
	Registry      string
	BuilderImage  string
	BuildEnvs     []string

	PipelineName    string
	PipelineRunName string
	PvcName         string
	SecretName      string

	// The branch or tag we are targeting with Pipelines (ie: main, refs/tags/*)
	PipelinesTargetBranch string

	// Static entries
	RepoUrl  string
	Revision string

	// Task references
	GitCloneTaskRef       string
	FuncBuildpacksTaskRef string
	FuncS2iTaskRef        string
	FuncDeployTaskRef     string
	FuncScaffoldTaskRef   string

	// Reference for build task - whether it should run after fetch-sources task or not
	RunAfterFetchSources string

	PipelineYamlURL string

	// S2I related properties
	S2iImageScriptsUrl string
}

// createPipelineTemplatePAC creates a Pipeline template used for PAC on-cluster build
// it creates the resource in the project directory
func createPipelineTemplatePAC(f fn.Function, labels map[string]string) error {
	data := templateData{
		FunctionName:         f.Name,
		Annotations:          f.Deploy.Annotations,
		Labels:               labels,
		PipelineName:         getPipelineName(f),
		RunAfterFetchSources: runAfterFetchSourcesRef,
		GitCloneTaskRef:      taskGitClonePACTaskRef,
	}

	for _, val := range []struct {
		ref   string
		field *string
	}{
		{getBuildpackTask(), &data.FuncBuildpacksTaskRef},
		{getS2ITask(), &data.FuncS2iTaskRef},
		{getDeployTask(), &data.FuncDeployTaskRef},
		{getScaffoldTask(), &data.FuncScaffoldTaskRef},
	} {
		ts, err := getTaskSpec(val.ref)
		if err != nil {
			return err
		}
		*val.field = ts
	}

	var template string
	if f.Build.Builder == builders.Pack {
		template = packPipelineTemplate
	} else if f.Build.Builder == builders.S2I {
		template = s2iPipelineTemplate
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}

	return createResource(f.Root, pipelineFileNamePAC, template, data)
}

// createPipelineRunTemplatePAC creates a PipelineRun template used for PAC on-cluster build
// it creates the resource in the project directory
func createPipelineRunTemplatePAC(f fn.Function, labels map[string]string) error {
	contextDir := f.Build.Git.ContextDir
	if contextDir == "" && f.Build.Builder == builders.S2I {
		// TODO(lkingland): could instead update S2I to interpret empty string
		// as cwd, such that builder-specific code can be kept out of here.
		contextDir = "."
	}

	pipelinesTargetBranch := f.Build.Git.Revision
	if pipelinesTargetBranch == "" {
		pipelinesTargetBranch = defaultPipelinesTargetBranch
	}

	buildEnvs := []string{}
	if len(f.Build.BuildEnvs) == 0 {
		buildEnvs = []string{"="}
	} else {
		for i := range f.Build.BuildEnvs {
			buildEnvs = append(buildEnvs, f.Build.BuildEnvs[i].KeyValuePair())
		}
	}

	s2iImageScriptsUrl := defaultS2iImageScriptsUrl
	if f.Runtime == "quarkus" {
		s2iImageScriptsUrl = quarkusS2iImageScriptsUrl
	}

	image := f.Deploy.Image
	if image == "" {
		image = f.Image
	}

	data := templateData{
		FunctionName:  f.Name,
		Annotations:   f.Deploy.Annotations,
		Labels:        labels,
		ContextDir:    contextDir,
		FunctionImage: image,
		Registry:      f.Registry,
		BuilderImage:  getBuilderImage(f),
		BuildEnvs:     buildEnvs,

		PipelineName:    getPipelineName(f),
		PipelineRunName: fmt.Sprintf("%s-run", getPipelineName(f)),
		PvcName:         getPipelinePvcName(f),
		SecretName:      getPipelineSecretName(f),

		PipelinesTargetBranch: pipelinesTargetBranch,

		GitCloneTaskRef: taskGitCloneRef,

		PipelineYamlURL: fmt.Sprintf("%s/%s", resourcesDirectory, pipelineFileNamePAC),

		S2iImageScriptsUrl: s2iImageScriptsUrl,

		RepoUrl:  "\"{{ repo_url }}\"",
		Revision: "\"{{ revision }}\"",
	}

	var template string
	if f.Build.Builder == builders.Pack {
		template = packRunTemplatePAC
	} else if f.Build.Builder == builders.S2I {
		template = s2iRunTemplatePAC
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}

	return createResource(f.Root, pipelineRunFilenamePAC, template, data)
}

// createResource creates a file in the input directory from the file template and a data
func createResource(projectRoot, fileName, fileTemplate string, data interface{}) error {
	tmpl, err := template.New(fileName).Parse(fileTemplate)
	if err != nil {
		return fmt.Errorf("error parsing pipeline template: %v", err)
	}

	if err = os.MkdirAll(path.Join(projectRoot, resourcesDirectory), os.ModePerm); err != nil {
		return fmt.Errorf("error creating pipeline resources path: %v", err)
	}

	filePath := path.Join(projectRoot, resourcesDirectory, fileName)

	overwrite := false
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		msg := fmt.Sprintf("There is already a file %q in the %q directory, do you want to overwrite it?", fileName, resourcesDirectory)
		if err := survey.AskOne(&survey.Confirm{Message: msg, Default: true}, &overwrite); err != nil {
			return err
		}
		if !overwrite {
			fmt.Printf(" ⚠️ Pipeline template is not updated in \"%s/%s\"\n", resourcesDirectory, fileName)
			return nil
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating pipeline resources: %v", err)
	}
	defer file.Close()

	err = tmpl.Execute(file, data)
	if err == nil {
		if overwrite {
			fmt.Printf(" ✅ Pipeline template is updated in \"%s/%s\"\n", resourcesDirectory, fileName)
		} else {
			fmt.Printf(" ✅ Pipeline template is created in \"%s/%s\"\n", resourcesDirectory, fileName)
		}
	}
	return err
}

// deleteAllPipelineTemplates deletes all templates and pipeline resources that exists for a function
// and are stored in the .tekton directory
func deleteAllPipelineTemplates(f fn.Function) string {
	err := os.RemoveAll(path.Join(f.Root, resourcesDirectory))
	if err != nil {
		return fmt.Sprintf("\n %v", err)
	}

	return ""
}

func getTaskSpec(taskYaml string) (string, error) {
	var err error
	var data map[string]any
	dec := yaml.NewDecoder(strings.NewReader(taskYaml))
	err = dec.Decode(&data)
	if err != nil {
		return "", err
	}
	data = map[string]any{
		"taskSpec": data["spec"],
	}
	var buff bytes.Buffer
	enc := yaml.NewEncoder(&buff)
	enc.SetIndent(2)
	err = enc.Encode(data)
	if err != nil {
		return "", err
	}
	err = enc.Close()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(buff.String(), "\n", "\n      "), nil
}

// createAndApplyPipelineTemplate creates and applies Pipeline template for a standard on-cluster build
// all resources are created on the fly, if there's a Pipeline defined in the project directory, it is used instead
func createAndApplyPipelineTemplate(f fn.Function, namespace string, labels map[string]string) error {
	// If Git is set up create fetch task and reference it from build task,
	// otherwise sources have been already uploaded to workspace PVC.
	gitCloneTaskRef := ""
	runAfterFetchSources := ""
	if f.Build.Git.URL != "" {
		runAfterFetchSources = runAfterFetchSourcesRef
		gitCloneTaskRef = taskGitCloneTaskRef
	}

	data := templateData{
		FunctionName:         f.Name,
		Annotations:          f.Deploy.Annotations,
		Labels:               labels,
		PipelineName:         getPipelineName(f),
		RunAfterFetchSources: runAfterFetchSources,
		GitCloneTaskRef:      gitCloneTaskRef,
	}

	for _, val := range []struct {
		ref   string
		field *string
	}{
		{getBuildpackTask(), &data.FuncBuildpacksTaskRef},
		{getS2ITask(), &data.FuncS2iTaskRef},
		{getDeployTask(), &data.FuncDeployTaskRef},
		{getScaffoldTask(), &data.FuncScaffoldTaskRef},
	} {
		ts, err := getTaskSpec(val.ref)
		if err != nil {
			return err
		}
		*val.field = ts
	}

	var template string
	if f.Build.Builder == builders.Pack {
		template = packPipelineTemplate
	} else if f.Build.Builder == builders.S2I {
		template = s2iPipelineTemplate
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}

	return createAndApplyResource(f.Root, pipelineFileName, template, "pipeline", getPipelineName(f), namespace, data)
}

// createAndApplyPipelineRunTemplate creates and applies PipelineRun template for a standard on-cluster build
// all resources are created on the fly, if there's a PipelineRun defined in the project directory, it is used instead
func createAndApplyPipelineRunTemplate(f fn.Function, namespace string, labels map[string]string) error {
	contextDir := f.Build.Git.ContextDir
	if contextDir == "" && f.Build.Builder == builders.S2I {
		// TODO(lkingland): could instead update S2I to interpret empty string
		// as cwd, such that builder-specific code can be kept out of here.
		contextDir = "."
	}

	pipelinesTargetBranch := f.Build.Git.Revision
	if pipelinesTargetBranch == "" {
		pipelinesTargetBranch = defaultPipelinesTargetBranch
	}

	buildEnvs := []string{}
	if len(f.Build.BuildEnvs) == 0 {
		buildEnvs = []string{"="}
	} else {
		for i := range f.Build.BuildEnvs {
			buildEnvs = append(buildEnvs, f.Build.BuildEnvs[i].KeyValuePair())
		}
	}

	s2iImageScriptsUrl := defaultS2iImageScriptsUrl
	if f.Runtime == "quarkus" {
		s2iImageScriptsUrl = quarkusS2iImageScriptsUrl
	}

	data := templateData{
		FunctionName:  f.Name,
		Annotations:   f.Deploy.Annotations,
		Labels:        labels,
		ContextDir:    contextDir,
		FunctionImage: f.Deploy.Image,
		Registry:      f.Registry,
		BuilderImage:  getBuilderImage(f),
		BuildEnvs:     buildEnvs,

		PipelineName:    getPipelineName(f),
		PipelineRunName: getPipelineRunGenerateName(f),
		PvcName:         getPipelinePvcName(f),
		SecretName:      getPipelineSecretName(f),

		S2iImageScriptsUrl: s2iImageScriptsUrl,

		RepoUrl:  f.Build.Git.URL,
		Revision: pipelinesTargetBranch,
	}

	var template string
	if f.Build.Builder == builders.Pack {
		template = packRunTemplate
	} else if f.Build.Builder == builders.S2I {
		template = s2iRunTemplate
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}

	return createAndApplyResource(f.Root, pipelineFileName, template, "pipelinerun", getPipelineRunGenerateName(f), namespace, data)
}

// allows simple mocking in unit tests
var manifestivalClient = k8s.GetManifestivalClient

// createAndApplyResource tries to create and apply a resource to the k8s cluster from the input template and data,
// if there's the same resource already created in the project directory, it is used instead
func createAndApplyResource(projectRoot, fileName, fileTemplate, kind, resourceName, namespace string, data interface{}) error {
	var source manifestival.Source

	filePath := path.Join(projectRoot, resourcesDirectory, fileName)
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		source = manifestival.Path(filePath)
	} else {
		tmpl, err := template.New("template").Parse(fileTemplate)
		if err != nil {
			return fmt.Errorf("error parsing template: %v", err)
		}

		var buf bytes.Buffer
		err = tmpl.Execute(&buf, data)
		if err != nil {
			return fmt.Errorf("error executing template: %v", err)
		}
		source = manifestival.Reader(&buf)
	}

	client, err := manifestivalClient()
	if err != nil {
		return fmt.Errorf("error generating template: %v", err)
	}

	m, err := manifestival.ManifestFrom(source, manifestival.UseClient(client))
	if err != nil {
		return fmt.Errorf("error generating template: %v", err)
	}

	resources := m.Resources()
	if len(resources) != 1 {
		return fmt.Errorf("error creating pipeline resources: there could be only a single resource in the template file %q", filePath)
	}

	if strings.ToLower(resources[0].GetKind()) != kind {
		return fmt.Errorf("error creating pipeline resources: expected resource kind in file %q is %q, but got %q", filePath, kind, resources[0].GetKind())
	}

	existingResourceName := resources[0].GetName()
	if kind == "pipelinerun" {
		existingResourceName = resources[0].GetGenerateName()
	}
	if existingResourceName != resourceName {
		return fmt.Errorf("error creating pipeline resources: expected resource name in file %q is %q, but got %q", filePath, resourceName, existingResourceName)
	}

	if resources[0].GetNamespace() != "" && resources[0].GetNamespace() != namespace {
		return fmt.Errorf("error creating pipeline resources: expected resource namespace in file %q is %q, but got %q", filePath, namespace, resources[0].GetNamespace())
	}

	m, err = m.Transform(manifestival.InjectNamespace(namespace))
	if err != nil {
		fmt.Printf("error procesing template: %v", err)
		return err
	}

	return m.Apply()
}
