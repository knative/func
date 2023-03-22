package tekton

import (
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/AlecAivazis/survey/v2"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
)

const (
	// Local resources properties
	resourcesDirectory  = ".tekton"
	pipelineFileName    = "pipeline.yaml"
	pipelineRunFilenane = "pipeline-run.yaml"

	// Tasks references
	taskGitCloneRef       = "git-clone"
	taskFuncS2iRef        = "https://raw.githubusercontent.com/knative/func/main/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml"
	taskFuncBuildpacksRef = "https://raw.githubusercontent.com/knative/func/main/pkg/pipelines/resources/tekton/task/func-buildpacks/0.1/func-buildpacks.yaml"
	taskFuncDeployRef     = "https://raw.githubusercontent.com/knative/func/main/pkg/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml"

	// S2I related properties
	defaultS2iImageScriptsUrl = "image:///usr/libexec/s2i"
	quarkusS2iImageScriptsUrl = "image:///usr/local/s2i"

	// The branch or tag we are targeting with Pipelines (ie: main, refs/tags/*)
	defaultPipelinesTargetBranch = "main"
)

type templateData struct {
	FunctionName  string
	Annotations   map[string]string
	Labels        []fn.Label
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

	PipelineYamlURL string

	// S2I related properties
	S2iImageScriptsUrl string
}

func createPipelineTemplate(f fn.Function) error {
	data := templateData{
		FunctionName: f.Name,
		Annotations:  f.Deploy.Annotations,
		Labels:       f.Deploy.Labels,
		PipelineName: getPipelineName(f),
	}

	if f.Build.Builder == builders.Pack {
		return createResource(f.Root, pipelineFileName, packPipelineTemplate, data)
	} else if f.Build.Builder == builders.S2I {
		return createResource(f.Root, pipelineFileName, s2iPipelineTemplate, data)
	}

	return fmt.Errorf("builder %q is not supported", f.Build.Builder)
}

func createPipelineRunTemplate(f fn.Function) error {
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
		Labels:        f.Deploy.Labels,
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
		FuncBuildpacksTaskRef: taskFuncBuildpacksRef,
		FuncS2iTaskRef:        taskFuncS2iRef,
		FuncDeployTaskRef:     taskFuncDeployRef,

		PipelineYamlURL: fmt.Sprintf("%s/%s", resourcesDirectory, pipelineFileName),

		S2iImageScriptsUrl: s2iImageScriptsUrl,

		RepoUrl:  "{{ repo_url }}",
		Revision: "{{ revision }}",
	}

	if f.Build.Builder == builders.Pack {
		return createResource(f.Root, pipelineRunFilenane, packRunTemplate, data)
	} else if f.Build.Builder == builders.S2I {
		return createResource(f.Root, pipelineRunFilenane, s2iRunTemplate, data)
	}

	return fmt.Errorf("builder %q is not supported", f.Build.Builder)
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
