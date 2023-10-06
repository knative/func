package tekton

import (
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
)

func GetS2IPipeline(f fn.Function) (*v1beta1.Pipeline, error) {

	labels, err := f.LabelsMap()
	if err != nil {
		return nil, fmt.Errorf("cannot generate labels: %w", err)
	}

	tasks := []v1beta1.PipelineTask{
		v1beta1.PipelineTask{
			Name: "fetch-sources",
			TaskRef: &v1beta1.TaskRef{
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "hub",
					Params: []v1beta1.Param{
						v1beta1.Param{
							Name: "kind",
							Value: v1beta1.ParamValue{
								Type:      "string",
								StringVal: "task",
							},
						},
						v1beta1.Param{
							Name: "name",
							Value: v1beta1.ParamValue{
								Type:      "string",
								StringVal: "git-clone",
							},
						},
						v1beta1.Param{
							Name: "version",
							Value: v1beta1.ParamValue{
								Type:      "string",
								StringVal: "0.4",
							},
						},
					},
				},
			},
			Params: []v1beta1.Param{
				v1beta1.Param{
					Name: "url",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.gitRepository)",
					},
				},
				v1beta1.Param{
					Name: "revision",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.gitRevision)",
					},
				},
			},
			Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
				v1beta1.WorkspacePipelineTaskBinding{
					Name:      "output",
					Workspace: "source-workspace",
				},
			},
		},
		v1beta1.PipelineTask{
			Name: "build",
			TaskSpec: &v1beta1.EmbeddedTask{
				TaskSpec: *S2ITask.Spec.DeepCopy(),
			},
			RunAfter: []string{"fetch-sources"},
			Params: []v1beta1.Param{
				v1beta1.Param{
					Name: "APP_IMAGE",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.imageName)",
					},
				},
				v1beta1.Param{
					Name: "REGISTRY",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.registry)",
					},
				},
				v1beta1.Param{
					Name: "PATH_CONTEXT",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.contextDir)",
					},
				},
				v1beta1.Param{
					Name: "BUILDER_IMAGE",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.builderImage)",
					},
				},
				v1beta1.Param{
					Name: "ENV_VARS",
					Value: v1beta1.ParamValue{
						Type: "array",
						ArrayVal: []string{
							"$(params.buildEnvs[*])",
						},
					},
				},
				v1beta1.Param{
					Name: "S2I_IMAGE_SCRIPTS_URL",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.s2iImageScriptsUrl)",
					},
				},
			},
			Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
				v1beta1.WorkspacePipelineTaskBinding{
					Name:      "source",
					Workspace: "source-workspace",
				},
				v1beta1.WorkspacePipelineTaskBinding{
					Name:      "cache",
					Workspace: "cache-workspace",
				},
				v1beta1.WorkspacePipelineTaskBinding{
					Name:      "dockerconfig",
					Workspace: "dockerconfig-workspace",
				},
			},
		},
		v1beta1.PipelineTask{
			Name: "deploy",
			TaskSpec: &v1beta1.EmbeddedTask{
				TaskSpec: *DeployTask.Spec.DeepCopy(),
			},
			RunAfter: []string{"build"},
			Params: []v1beta1.Param{
				v1beta1.Param{
					Name: "path",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(workspaces.source.path)/$(params.contextDir)",
					},
				},
				v1beta1.Param{
					Name: "image",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: "$(params.imageName)@$(tasks.build.results.IMAGE_DIGEST)",
					},
				},
			},
			Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
				v1beta1.WorkspacePipelineTaskBinding{
					Name:      "source",
					Workspace: "source-workspace",
				},
			},
		},
	}

	if f.Build.Git.URL == "" {
		tasks = tasks[1:]
		tasks[0].RunAfter = nil
	}

	result := v1beta1.Pipeline{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        getPipelineName(f),
			Labels:      labels,
			Annotations: f.Deploy.Annotations,
		},
		Spec: v1beta1.PipelineSpec{
			Tasks: tasks,
			Params: []v1beta1.ParamSpec{
				v1beta1.ParamSpec{
					Name:        "gitRepository",
					Type:        "string",
					Description: "Git repository that hosts the function project",
					Default: &v1beta1.ParamValue{
						Type: "string",
					},
				},
				v1beta1.ParamSpec{
					Name:        "gitRevision",
					Type:        "string",
					Description: "Git revision to build",
				},
				v1beta1.ParamSpec{
					Name:        "contextDir",
					Type:        "string",
					Description: "Path where the function project is",
					Default: &v1beta1.ParamValue{
						Type: "string",
					},
				},
				v1beta1.ParamSpec{
					Name:        "imageName",
					Type:        "string",
					Description: "Function image name",
				},
				v1beta1.ParamSpec{
					Name:        "registry",
					Type:        "string",
					Description: "The registry associated with the function image",
				},
				v1beta1.ParamSpec{
					Name:        "builderImage",
					Type:        "string",
					Description: "Builder image to be used",
				},
				v1beta1.ParamSpec{
					Name:        "buildEnvs",
					Type:        "array",
					Description: "Environment variables to set during build time",
				},
				v1beta1.ParamSpec{
					Name:        "s2iImageScriptsUrl",
					Type:        "string",
					Description: "URL containing the default assemble and run scripts for the builder image",
					Default: &v1beta1.ParamValue{
						Type:      "string",
						StringVal: "image:///usr/libexec/s2i",
					},
				},
			},
			Workspaces: []v1beta1.PipelineWorkspaceDeclaration{
				v1beta1.PipelineWorkspaceDeclaration{
					Name:        "source-workspace",
					Description: "Directory where function source is located.",
				},
				v1beta1.PipelineWorkspaceDeclaration{
					Name:        "cache-workspace",
					Description: "Directory where build cache is stored.",
				},
				v1beta1.PipelineWorkspaceDeclaration{
					Name:        "dockerconfig-workspace",
					Description: "Directory containing image registry credentials stored in config.json file.",
					Optional:    true,
				},
			},
		},
	}

	return &result, nil
}

func GetS2IPipelineRun(f fn.Function) (*v1beta1.PipelineRun, error) {
	labels, err := f.LabelsMap()
	if err != nil {
		return nil, fmt.Errorf("cannot generate labels: %w", err)
	}
	labels["tekton.dev/pipeline"] = getPipelineName(f)

	pipelinesTargetBranch := f.Build.Git.Revision
	if pipelinesTargetBranch == "" {
		pipelinesTargetBranch = defaultPipelinesTargetBranch
	}

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

	s2iImageScriptsUrl := defaultS2iImageScriptsUrl
	if f.Runtime == "quarkus" {
		s2iImageScriptsUrl = quarkusS2iImageScriptsUrl
	}

	result := v1beta1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PipelineRun",
			APIVersion: "tekton.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: getPipelineRunGenerateName(f),
			Labels:       labels,
			Annotations:  f.Deploy.Annotations,
		},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{
				Name: getPipelineName(f),
			},
			Params: []v1beta1.Param{
				v1beta1.Param{
					Name: "gitRepository",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: f.Build.Git.URL,
					},
				},
				v1beta1.Param{
					Name: "gitRevision",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: pipelinesTargetBranch,
					},
				},
				v1beta1.Param{
					Name: "contextDir",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: contextDir,
					},
				},
				v1beta1.Param{
					Name: "imageName",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: f.Image,
					},
				},
				v1beta1.Param{
					Name: "registry",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: f.Registry,
					},
				},
				v1beta1.Param{
					Name: "builderImage",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: getBuilderImage(f),
					},
				},
				v1beta1.Param{
					Name: "buildEnvs",
					Value: v1beta1.ParamValue{
						Type:     "array",
						ArrayVal: buildEnvs,
					},
				},
				v1beta1.Param{
					Name: "s2iImageScriptsUrl",
					Value: v1beta1.ParamValue{
						Type:      "string",
						StringVal: s2iImageScriptsUrl,
					},
				},
			},
			Workspaces: []v1beta1.WorkspaceBinding{
				v1beta1.WorkspaceBinding{
					Name:    "source-workspace",
					SubPath: "source",
					PersistentVolumeClaim: &coreV1.PersistentVolumeClaimVolumeSource{
						ClaimName: getPipelinePvcName(f),
					},
				},
				v1beta1.WorkspaceBinding{
					Name:    "cache-workspace",
					SubPath: "cache",
					PersistentVolumeClaim: &coreV1.PersistentVolumeClaimVolumeSource{
						ClaimName: getPipelinePvcName(f),
					},
				},
				v1beta1.WorkspaceBinding{
					Name: "dockerconfig-workspace",
					Secret: &coreV1.SecretVolumeSource{
						SecretName: getPipelineSecretName(f),
					},
				},
			},
		},
	}
	return &result, nil
}

const (
	// s2iPipelineTemplate contains the S2I template used for both Tekton standard and PAC Pipeline
	s2iPipelineTemplate = `
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  labels:
    {{range $key, $value := .Labels -}}
     "{{$key}}": "{{$value}}"
    {{end}}
  annotations:
    {{range $key, $value := .Annotations -}}
     "{{$key}}": "{{$value}}"
    {{end}}
  name: {{.PipelineName}}
spec:
  params:
    - default: ''
      description: Git repository that hosts the function project
      name: gitRepository
      type: string
    - description: Git revision to build
      name: gitRevision
      type: string
    - default: ''
      description: Path where the function project is
      name: contextDir
      type: string
    - description: Function image name
      name: imageName
      type: string
    - description: The registry associated with the function image
      name: registry
      type: string
    - description: Builder image to be used
      name: builderImage
      type: string
    - description: Environment variables to set during build time
      name: buildEnvs
      type: array
    - description: URL containing the default assemble and run scripts for the builder image
      name: s2iImageScriptsUrl
      type: string
      default: 'image:///usr/libexec/s2i'
  tasks:
    {{.GitCloneTaskRef}}
    - name: build
      params:
        - name: APP_IMAGE
          value: $(params.imageName)
        - name: REGISTRY
          value: $(params.registry)
        - name: PATH_CONTEXT
          value: $(params.contextDir)
        - name: BUILDER_IMAGE
          value: $(params.builderImage)
        - name: ENV_VARS
          value:
            - '$(params.buildEnvs[*])'
        - name: S2I_IMAGE_SCRIPTS_URL
          value: $(params.s2iImageScriptsUrl)
      {{.RunAfterFetchSources}}
      {{.FuncS2iTaskRef}}
      workspaces:
        - name: source
          workspace: source-workspace
        - name: cache
          workspace: cache-workspace
        - name: dockerconfig
          workspace: dockerconfig-workspace
    - name: deploy
      params:
        - name: path
          value: $(workspaces.source.path)/$(params.contextDir)
        - name: image
          value: $(params.imageName)@$(tasks.build.results.IMAGE_DIGEST)
      runAfter:
        - build
      {{.FuncDeployTaskRef}}
      workspaces:
        - name: source
          workspace: source-workspace
  workspaces:
    - description: Directory where function source is located.
      name: source-workspace
    - description: Directory where build cache is stored.
      name: cache-workspace
    - description: Directory containing image registry credentials stored in config.json file.
      name: dockerconfig-workspace
      optional: true
`
	// s2iRunTemplate contains the S2I template used for Tekton standard PipelineRun
	s2iRunTemplate = `
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  labels:
    {{range $key, $value := .Labels -}}
     "{{$key}}": "{{$value}}"
    {{end}}
    tekton.dev/pipeline: {{.PipelineName}}
  annotations:
    # User defined Annotations
    {{range $key, $value := .Annotations -}}
     "{{$key}}": "{{$value}}"
    {{end}}
  generateName: {{.PipelineRunName}}
spec:
  params:
    - name: gitRepository
      value: {{.RepoUrl}}
    - name: gitRevision
      value: {{.Revision}}
    - name: contextDir
      value: {{.ContextDir}}
    - name: imageName
      value: {{.FunctionImage}}
    - name: registry
      value: {{.Registry}}
    - name: builderImage
      value: {{.BuilderImage}}
    - name: buildEnvs
      value:
        {{range .BuildEnvs -}}
           - {{.}}
        {{end}}
    - name: s2iImageScriptsUrl
      value: {{.S2iImageScriptsUrl}}
  pipelineRef:
   name: {{.PipelineName}}
  workspaces:
    - name: source-workspace
      persistentVolumeClaim:
        claimName: {{.PvcName}}
      subPath: source
    - name: cache-workspace
      persistentVolumeClaim:
        claimName: {{.PvcName}}
      subPath: cache
    - name: dockerconfig-workspace
      secret:
        secretName: {{.SecretName}}
`
	// s2iRunTemplatePAC contains the S2I template used for Tekton PAC PipelineRun
	s2iRunTemplatePAC = `
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  labels:
    {{range $key, $value := .Labels -}}
     "{{$key}}": "{{$value}}"
    {{end}}
    tekton.dev/pipeline: {{.PipelineName}}
  annotations:
    # The event we are targeting as seen from the webhook payload
    # this can be an array too, i.e: [pull_request, push]
    pipelinesascode.tekton.dev/on-event: "[push]"

    # The branch or tag we are targeting (ie: main, refs/tags/*)
    pipelinesascode.tekton.dev/on-target-branch: "[{{.PipelinesTargetBranch}}]"

    # Fetch the git-clone task from hub
    pipelinesascode.tekton.dev/task: {{.GitCloneTaskRef}}

    # Fetch the func-s2i task
    pipelinesascode.tekton.dev/task-1: {{.FuncS2iTaskRef}}

    # Fetch the func-deploy task
    pipelinesascode.tekton.dev/task-2: {{.FuncDeployTaskRef}}

    # Fetch the pipelie definition from the .tekton directory
    pipelinesascode.tekton.dev/pipeline: {{.PipelineYamlURL}}

    # How many runs we want to keep attached to this event
    pipelinesascode.tekton.dev/max-keep-runs: "5"

    # User defined Annotations
    {{range $key, $value := .Annotations -}}
     "{{$key}}": "{{$value}}"
    {{end}}
  generateName: {{.PipelineRunName}}
spec:
  params:
    - name: gitRepository
      value: {{.RepoUrl}}
    - name: gitRevision
      value: {{.Revision}}
    - name: contextDir
      value: {{.ContextDir}}
    - name: imageName
      value: {{.FunctionImage}}
    - name: registry
      value: {{.Registry}}
    - name: builderImage
      value: {{.BuilderImage}}
    - name: buildEnvs
      value:
        {{range .BuildEnvs -}}
           - {{.}}
        {{end}}
    - name: s2iImageScriptsUrl
      value: {{.S2iImageScriptsUrl}}
  pipelineRef:
   name: {{.PipelineName}}
  workspaces:
    - name: source-workspace
      persistentVolumeClaim:
        claimName: {{.PvcName}}
      subPath: source
    - name: cache-workspace
      persistentVolumeClaim:
        claimName: {{.PvcName}}
      subPath: cache
    - name: dockerconfig-workspace
      secret:
        secretName: {{.SecretName}}
`
)
