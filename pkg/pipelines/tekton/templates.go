package tekton

import (
	"context"
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
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

	// Following section contains references for Tasks to be used in Pipeline templates,
	// there is a difference if we use PAC approach or standard Tekton approach.
	//
	// This can be simplified once we start consuming tasks from Tekton Hub
	taskFuncBuildpacksPACTaskRef = `taskRef:
        kind: Task
        name: func-buildpacks`
	taskFuncS2iPACTaskRef = `taskRef:
        kind: Task
        name: func-s2i`
	taskFuncDeployPACTaskRef = `taskRef:
        kind: Task
        name: func-deploy`

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

var (
	FuncRepoRef       = "knative/func"
	FuncRepoBranchRef = "main"

	taskBasePath = "https://raw.githubusercontent.com/" +
		FuncRepoRef + "/" + FuncRepoBranchRef + "/pkg/pipelines/resources/tekton/task/"
	BuildpackTaskURL = taskBasePath + "func-buildpacks/0.2/func-buildpacks.yaml"
	S2ITaskURL       = taskBasePath + "func-s2i/0.2/func-s2i.yaml"
	DeployTaskURL    = taskBasePath + "func-deploy/0.1/func-deploy.yaml"
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
		FunctionName:          f.Name,
		Annotations:           f.Deploy.Annotations,
		Labels:                labels,
		PipelineName:          getPipelineName(f),
		RunAfterFetchSources:  runAfterFetchSourcesRef,
		GitCloneTaskRef:       taskGitClonePACTaskRef,
		FuncBuildpacksTaskRef: taskFuncBuildpacksPACTaskRef,
		FuncS2iTaskRef:        taskFuncS2iPACTaskRef,
		FuncDeployTaskRef:     taskFuncDeployPACTaskRef,
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

	data := templateData{
		FunctionName:  f.Name,
		Annotations:   f.Deploy.Annotations,
		Labels:        labels,
		ContextDir:    contextDir,
		FunctionImage: f.Image,
		Registry:      f.Registry,
		BuilderImage:  getBuilderImage(f),
		BuildEnvs:     buildEnvs,

		PipelineName:    getPipelineName(f),
		PipelineRunName: fmt.Sprintf("%s-run", getPipelineName(f)),
		PvcName:         getPipelinePvcName(f),
		SecretName:      getPipelineSecretName(f),

		PipelinesTargetBranch: pipelinesTargetBranch,

		GitCloneTaskRef:       taskGitCloneRef,
		FuncBuildpacksTaskRef: BuildpackTaskURL,
		FuncS2iTaskRef:        S2ITaskURL,
		FuncDeployTaskRef:     DeployTaskURL,

		PipelineYamlURL: fmt.Sprintf("%s/%s", resourcesDirectory, pipelineFileNamePAC),

		S2iImageScriptsUrl: s2iImageScriptsUrl,

		RepoUrl:  "{{ repo_url }}",
		Revision: "{{ revision }}",
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

// createAndApplyPipelineTemplate creates and applies Pipeline template for a standard on-cluster build
// all resources are created on the fly, if there's a Pipeline defined in the project directory, it is used instead
func createAndApplyPipelineTemplate(f fn.Function, namespace string, labels map[string]string) error {
	if f.Build.Builder == builders.Pack || f.Build.Builder == builders.S2I {
		iface, err := newTektonClient()
		if err != nil {
			return fmt.Errorf("cannot create tekton client: %w", err)
		}
		pipeline, err := getPipeline(f, labels)
		if err != nil {
			return fmt.Errorf("cannot generate pipeline: %w", err)
		}
		_, err = iface.TektonV1beta1().Pipelines(namespace).Create(context.TODO(), pipeline, v1.CreateOptions{})
		if err != nil {
			err = fmt.Errorf("cannot create pipeline in cluster: %w", err)
		}
		return err
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}
}

// createAndApplyPipelineRunTemplate creates and applies PipelineRun template for a standard on-cluster build
// all resources are created on the fly, if there's a PipelineRun defined in the project directory, it is used instead
func createAndApplyPipelineRunTemplate(f fn.Function, namespace string, labels map[string]string) error {
	if f.Build.Builder == builders.Pack || f.Build.Builder == builders.S2I {
		iface, err := newTektonClient()
		if err != nil {
			return err
		}
		piplineRun, err := getPipelineRun(f, labels)
		if err != nil {
			return fmt.Errorf("cannot generate pipeline run: %w", err)
		}
		_, err = iface.TektonV1beta1().PipelineRuns(namespace).Create(context.Background(), piplineRun, v1.CreateOptions{})
		if err != nil {
			err = fmt.Errorf("cannot create pipeline run in cluster: %w", err)
		}
		return err
	} else {
		return builders.ErrBuilderNotSupported{Builder: f.Build.Builder}
	}
}

// allows simple mocking in unit tests
var newTektonClient func() (versioned.Interface, error) = func() (versioned.Interface, error) {
	cli, err := NewTektonClients()
	if err != nil {
		return nil, err
	}
	return cli.Tekton, nil
}
