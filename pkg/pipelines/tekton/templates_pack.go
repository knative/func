package tekton

const (
	// packPipelineTemplate contains the Buildpacks template used for both Tekton standard and PAC Pipeline
	packPipelineTemplate = `
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
  tasks:
    {{.GitCloneTaskRef}}
    - name: build
      params:
        - name: APP_IMAGE
          value: $(params.imageName)
        - name: REGISTRY
          value: $(params.registry)
        - name: SOURCE_SUBPATH
          value: $(params.contextDir)
        - name: BUILDER_IMAGE
          value: $(params.builderImage)
        - name: ENV_VARS
          value:
            - '$(params.buildEnvs[*])'
      {{.RunAfterFetchSources}}
      {{.FuncBuildpacksTaskRef}}
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

	// packRunTemplate contains the Buildpacks template used for Tekton standard PipelineRun
	packRunTemplate = `
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
	// packRunTemplatePAC contains the Buildpacks template used for the Tekton PAC PipelineRun
	packRunTemplatePAC = `
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

    # Fetch the func-buildpacks task
    pipelinesascode.tekton.dev/task-1: {{.FuncBuildpacksTaskRef}}

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
