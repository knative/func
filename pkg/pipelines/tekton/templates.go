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
	resourcesDirectory  = ".tekton"
	pipelineFileName    = "pipeline.yaml"
	pipelineRunFilenane = "pipeline-run.yaml"
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

	// Static entries
	RepoUrl  string
	Revision string
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

	buildEnvs := []string{}
	if len(f.Build.BuildEnvs) == 0 {
		buildEnvs = []string{"="}
	} else {
		for i := range f.Build.BuildEnvs {
			buildEnvs = append(buildEnvs, f.Build.BuildEnvs[i].KeyValuePair())
		}
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

		RepoUrl:  "{{ repo_url }}",
		Revision: "{{ revision }}",
	}

	if f.Build.Builder == builders.Pack {
		return createResource(f.Root, pipelineRunFilenane, packRunTemplate, data)
	}

	return fmt.Errorf("builder %q is not supported", f.Build.Builder)
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
