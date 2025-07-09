package tekton

func GetDevConsolePipelines() string {
	return GetNodeJSPipeline()
}

func GetNodeJSPipeline() string {
	return `apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: devconsole-nodejs-function-pipeline
  namespace: openshift
  labels:
    function.knative.dev: "true"
    function.knative.dev/name: viewer
    function.knative.dev/runtime: nodejs
spec:
  params:
    - description: Git repository that hosts the function project
      name: GIT_REPO
      type: string
    - description: Git revision to build
      name: GIT_REVISION
      type: string
    - description: Path where the function project is
      name: PATH_CONTEXT
      type: string
      default: .
    - description: Function image name
      name: IMAGE_NAME
      type: string
    - description: Builder image to be used
      name: BUILDER_IMAGE
      type: string
      default: image-registry.openshift-image-registry.svc:5000/openshift/nodejs:16-ubi8
    - description: Environment variables to set during build time
      name: BUILD_ENVS
      type: array
      default: []
    - default: 'image:///usr/libexec/s2i'
      description: >-
        URL containing the default assemble and run scripts for the builder
        image.
      name: s2iImageScriptsUrl
      type: string
  resources: []
  workspaces:
    - description: Directory where function source is located.
      name: source-workspace
  tasks:
    - name: fetch-sources
      params:
        - name: url
          value: $(params.GIT_REPO)
        - name: revision
          value: $(params.GIT_REVISION)
      taskRef:
        kind: ClusterTask
        name: git-clone
      workspaces:
        - name: output
          workspace: source-workspace
    - name: build
      params:
        - name: IMAGE
          value: $(params.IMAGE_NAME)
        - name: PATH_CONTEXT
          value: $(params.PATH_CONTEXT)
        - name: BUILDER_IMAGE
          value: $(params.BUILDER_IMAGE)
        - name: ENV_VARS
          value:
            - '$(params.BUILD_ENVS[*])'
        - name: S2I_IMAGE_SCRIPTS_URL
          value: $(params.s2iImageScriptsUrl)
      runAfter:
        - fetch-sources
      taskRef:
        kind: ClusterTask
        name: func-s2i
      workspaces:
        - name: source
          workspace: source-workspace
    - name: deploy
      params:
        - name: path
          value: $(workspaces.source.path)/$(params.PATH_CONTEXT)
        - name: image
          value: $(params.IMAGE_NAME)@$(tasks.build.results.IMAGE_DIGEST)
      runAfter:
        - build
      taskRef:
        kind: ClusterTask
        name: func-deploy
      workspaces:
        - name: source
          workspace: source-workspace
  finally: []
`
}
