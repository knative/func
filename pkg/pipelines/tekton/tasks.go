package tekton

import (
	"fmt"
	"strings"
)

var DeployerImage = "ghcr.io/knative/func-utils:v2"

func getBuildpackTask() string {
	return `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: func-buildpacks
  labels:
    app.kubernetes.io/version: "0.1"
  annotations:
    tekton.dev/categories: Image Build
    tekton.dev/pipelines.minVersion: "0.17.0"
    tekton.dev/tags: image-build
    tekton.dev/displayName: "Knative Functions Buildpacks"
    tekton.dev/platforms: "linux/amd64"
spec:
  description: >-
    The Knative Functions Buildpacks task builds source into a container image and pushes it to a registry,
    using Cloud Native Buildpacks. This task is based on the Buildpacks Tekton task v 0.4.

  workspaces:
    - name: source
      description: Directory where application source is located.
    - name: cache
      description: Directory where cache is stored (when no cache image is provided).
      optional: true
    - name: dockerconfig
      description: >-
        An optional workspace that allows providing a .docker/config.json file
        for Buildpacks lifecycle binary to access the container registry.
        The file should be placed at the root of the Workspace with name config.json.
      optional: true

  params:
    - name: APP_IMAGE
      description: The name of where to store the app image.
    - name: REGISTRY
      description: The registry associated with the function image.
    - name: BUILDER_IMAGE
      description: The image on which builds will run (must include lifecycle and compatible buildpacks).
    - name: SOURCE_SUBPATH
      description: A subpath within the "source" input where the source to build is located.
      default: ""
    - name: ENV_VARS
      type: array
      description: Environment variables to set during _build-time_.
      default: []
    - name: RUN_IMAGE
      description: Reference to a run image to use.
      default: ""
    - name: CACHE_IMAGE
      description: The name of the persistent app cache image (if no cache workspace is provided).
      default: ""
    - name: SKIP_RESTORE
      description: Do not write layer metadata or restore cached layers.
      default: "false"
    - name: USER_ID
      description: The user ID of the builder image user.
      default: "1001"
    - name: GROUP_ID
      description: The group ID of the builder image user.
      default: "0"
      ##############################################################
      #####  "default" has been changed to "0" for Knative Functions
    - name: PLATFORM_DIR
      description: The name of the platform directory.
      default: empty-dir

  results:
    - name: IMAGE_DIGEST
      description: The digest of the built "APP_IMAGE".

  stepTemplate:
    env:
      - name: CNB_PLATFORM_API
        value: "0.10"

  steps:
    - name: prepare
      image: docker.io/library/bash:5.1.4@sha256:b208215a4655538be652b2769d82e576bc4d0a2bb132144c060efc5be8c3f5d6
      args:
        - "--env-vars"
        - "$(params.ENV_VARS[*])"
      script: |
        #!/usr/bin/env bash
        set -e

        if [[ "$(workspaces.cache.bound)" == "true" ]]; then
          echo "> Setting permissions on '$(workspaces.cache.path)'..."
          chown -R "$(params.USER_ID):$(params.GROUP_ID)" "$(workspaces.cache.path)"
        fi

        #######################################################
        #####  "/emptyDir" has been added for Knative Functions
        for path in "/tekton/home" "/layers" "/emptyDir" "$(workspaces.source.path)"; do
          echo "> Setting permissions on '$path'..."
          chown -R "$(params.USER_ID):$(params.GROUP_ID)" "$path"

          if [[ "$path" == "$(workspaces.source.path)" ]]; then
              chmod 775 "$(workspaces.source.path)"
          fi
        done

        echo "> Parsing additional configuration..."
        parsing_flag=""
        envs=()
        for arg in "$@"; do
            if [[ "$arg" == "--env-vars" ]]; then
                echo "-> Parsing env variables..."
                parsing_flag="env-vars"
            elif [[ "$parsing_flag" == "env-vars" ]]; then
                envs+=("$arg")
            fi
        done

        echo "> Processing any environment variables..."
        ENV_DIR="/platform/env"

        echo "--> Creating 'env' directory: $ENV_DIR"
        mkdir -p "$ENV_DIR"

        for env in "${envs[@]}"; do
            IFS='=' read -r key value <<< "$env"
            if [[ "$key" != "" && "$value" != "" ]]; then
                path="${ENV_DIR}/${key}"
                echo "--> Writing ${path}..."
                echo -n "$value" > "$path"
            fi
        done

        ############################################
        ##### Added part for Knative Functions #####
        ############################################

        func_file="$(workspaces.source.path)/func.yaml"
        if [ "$(params.SOURCE_SUBPATH)" != "" ]; then
          func_file="$(workspaces.source.path)/$(params.SOURCE_SUBPATH)/func.yaml"
        fi
        echo "--> Saving 'func.yaml'"
        cp $func_file /emptyDir/func.yaml

        ############################################

      volumeMounts:
        - name: layers-dir
          mountPath: /layers
        - name: $(params.PLATFORM_DIR)
          mountPath: /platform
          ########################################################
          #####   "/emptyDir" has been added for Knative Functions
        - name: empty-dir
          mountPath: /emptyDir

    - name: create
      image: $(params.BUILDER_IMAGE)
      imagePullPolicy: Always
      command: ["/cnb/lifecycle/creator"]
      env:
        - name: DOCKER_CONFIG
          value: $(workspaces.dockerconfig.path)
      args:
        - "-app=$(workspaces.source.path)/$(params.SOURCE_SUBPATH)"
        - "-cache-dir=$(workspaces.cache.path)"
        - "-cache-image=$(params.CACHE_IMAGE)"
        - "-uid=$(params.USER_ID)"
        - "-gid=$(params.GROUP_ID)"
        - "-layers=/layers"
        - "-platform=/platform"
        - "-report=/layers/report.toml"
        - "-skip-restore=$(params.SKIP_RESTORE)"
        - "-previous-image=$(params.APP_IMAGE)"
        - "-run-image=$(params.RUN_IMAGE)"
        - "$(params.APP_IMAGE)"
      volumeMounts:
        - name: layers-dir
          mountPath: /layers
        - name: $(params.PLATFORM_DIR)
          mountPath: /platform
      securityContext:
        runAsUser: 1001
        #################################################################
        #####  "runAsGroup" has been changed to "0" for Knative Functions
        runAsGroup: 0

    - name: results
      image: docker.io/library/bash:5.1.4@sha256:b208215a4655538be652b2769d82e576bc4d0a2bb132144c060efc5be8c3f5d6
      script: |
        #!/usr/bin/env bash
        set -e
        cat /layers/report.toml | grep "digest" | cut -d'"' -f2 | cut -d'"' -f2 | tr -d '\n' | tee $(results.IMAGE_DIGEST.path)

        ############################################
        ##### Added part for Knative Functions #####
        ############################################

        digest=$(cat $(results.IMAGE_DIGEST.path))

        func_file="$(workspaces.source.path)/func.yaml"
        if [ "$(params.SOURCE_SUBPATH)" != "" ]; then
          func_file="$(workspaces.source.path)/$(params.SOURCE_SUBPATH)/func.yaml"
        fi

        if [[ ! -f "$func_file" ]]; then
          echo "--> Restoring 'func.yaml'"
          mkdir -p "$(workspaces.source.path)/$(params.SOURCE_SUBPATH)"
          cp /emptyDir/func.yaml $func_file
        fi

        echo ""
        sed -i "s|^image:.*$|image: $(params.APP_IMAGE)|" "$func_file"
        echo "Function image name: $(params.APP_IMAGE)"

        sed -i "s/^imageDigest:.*$/imageDigest: $digest/" "$func_file"
        echo "Function image digest: $digest"

        sed -i "s|^registry:.*$|registry: $(params.REGISTRY)|" "$func_file"
        echo "Function image registry: $(params.REGISTRY)"

        ############################################
      volumeMounts:
        - name: layers-dir
          mountPath: /layers
          ########################################################
          #####   "/emptyDir" has been added for Knative Functions
        - name: empty-dir
          mountPath: /emptyDir

  volumes:
    - name: empty-dir
      emptyDir: {}
    - name: layers-dir
      emptyDir: {}
`
}

func getS2ITask() string {
	return fmt.Sprintf(`apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: func-s2i
  labels:
    app.kubernetes.io/version: "0.1"
  annotations:
    tekton.dev/pipelines.minVersion: "0.17.0"
    tekton.dev/categories: Image Build
    tekton.dev/tags: image-build
    tekton.dev/platforms: "linux/amd64"
spec:
  description: >-
    Knative Functions Source-to-Image (S2I) is a toolkit and workflow for building reproducible
    container images from source code

    S2I produces images by injecting source code into a base S2I container image
    and letting the container prepare that source code for execution. The base
    S2I container images contains the language runtime and build tools needed for
    building and running the source code.

  params:
    - name: BUILDER_IMAGE
      description: The location of the s2i builder image.
    - name: IMAGE
      description: Reference of the image S2I will produce.
    - name: REGISTRY
      description: The registry associated with the function image.
      default: ""
    - name: PATH_CONTEXT
      description: The location of the path to run s2i from.
      default: .
    - name: TLSVERIFY
      description: Verify the TLS on the registry endpoint (for push/pull to a non-TLS registry)
      default: "true"
    - name: LOGLEVEL
      description: Log level when running the S2I binary
      default: "0"
    - name: ENV_VARS
      type: array
      description: Environment variables to set during _build-time_.
      default: []
    - name: S2I_IMAGE_SCRIPTS_URL
      description: The URL containing the default assemble and run scripts for the builder image.
      default: "image:///usr/libexec/s2i"
  workspaces:
    - name: source
    - name: cache
      description: Directory where cache is stored (e.g. local mvn repo).
      optional: true
    - name: sslcertdir
      optional: true
    - name: dockerconfig
      description: >-
        An optional workspace that allows providing a .docker/config.json file
        for Buildah to access the container registry.
        The file should be placed at the root of the Workspace with name config.json.
      optional: true
  results:
    - name: IMAGE_DIGEST
      description: Digest of the image just built.
  steps:
    - name: generate
      image: %s
      workingDir: $(workspaces.source.path)
      command:
        - s2i-generate
        - "--target"
        - /gen-source
        - "--path-context"
        - $(params.PATH_CONTEXT)
        - "--builder-image"
        - $(params.BUILDER_IMAGE)
        - "--registry"
        - $(params.REGISTRY)
        - "--image-script-url"
        - $(params.S2I_IMAGE_SCRIPTS_URL)
        - "--log-level"
        - $(params.LOGLEVEL)
        - $(params.ENV_VARS[*])
      volumeMounts:
        - mountPath: /gen-source
          name: gen-source
        - mountPath: /env-vars
          name: env-vars
    - name: build
      image: quay.io/buildah/stable:v1.31.0
      workingDir: /gen-source
      script: |
        TLS_VERIFY_FLAG=""
        if [ "$(params.TLSVERIFY)" = "false" ] || [ "$(params.TLSVERIFY)" = "0" ]; then
          TLS_VERIFY_FLAG="--tls-verify=false"
        fi

        # Set certificate directory flag if workspace is bound
        [[ "$(workspaces.sslcertdir.bound)" == "true" ]] && CERT_DIR_FLAG="--cert-dir $(workspaces.sslcertdir.path)"

        # Set docker config before any buildah commands
        [[ "$(workspaces.dockerconfig.bound)" == "true" ]] && export DOCKER_CONFIG="$(workspaces.dockerconfig.path)"

        # Setup artifacts cache path
        ARTIFACTS_CACHE_PATH="$(workspaces.cache.path)/mvn-artifacts"
        [ -d "${ARTIFACTS_CACHE_PATH}" ] || mkdir "${ARTIFACTS_CACHE_PATH}"

        # Build the image
        buildah ${CERT_DIR_FLAG} bud --storage-driver=vfs ${TLS_VERIFY_FLAG} --layers \
          -v "${ARTIFACTS_CACHE_PATH}:/tmp/artifacts/:rw,z,U" \
          -f /gen-source/Dockerfile.gen -t $(params.IMAGE) .

        # Push the image
        buildah ${CERT_DIR_FLAG} push --storage-driver=vfs ${TLS_VERIFY_FLAG} --digestfile $(workspaces.source.path)/image-digest \
          $(params.IMAGE) docker://$(params.IMAGE)

        # Output the image digest
        cat $(workspaces.source.path)/image-digest | tee /tekton/results/IMAGE_DIGEST
      volumeMounts:
      - name: varlibcontainers
        mountPath: /var/lib/containers
      - mountPath: /gen-source
        name: gen-source
      securityContext:
        capabilities:
          add: ["SETFCAP"]
  volumes:
    - emptyDir: {}
      name: varlibcontainers
    - emptyDir: {}
      name: gen-source
    - emptyDir: {}
      name: env-vars
`, DeployerImage)
}

func getDeployTask() string {
	return fmt.Sprintf(`apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: func-deploy
  labels:
    app.kubernetes.io/version: "0.1"
  annotations:
    tekton.dev/pipelines.minVersion: "0.12.1"
    tekton.dev/categories: CLI
    tekton.dev/tags: cli
    tekton.dev/platforms: "linux/amd64"
spec:
  description: >-
    This Task performs a deploy operation using the Knative "func"" CLI
  params:
    - name: path
      description: Path to the function project
      default: ""
    - name: image
      description: Container image to be deployed
      default: ""
  workspaces:
    - name: source
      description: The workspace containing the function project
  steps:
    - name: func-deploy
      image: "%s"
      command: ["deploy", "$(params.path)", "$(params.image)"]
`, DeployerImage)
}

func getScaffoldTask() string {
	return fmt.Sprintf(`apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: func-scaffold
  labels:
    app.kubernetes.io/version: "0.1"
  annotations:
    tekton.dev/pipelines.minVersion: "0.12.1"
    tekton.dev/categories: CLI
    tekton.dev/tags: cli
    tekton.dev/platforms: "linux/amd64"
spec:
  params:
    - name: path
      description: Path to the function project
      default: ""
  workspaces:
    - name: source
      description: The workspace containing the function project
  steps:
    - name: func-scaffold
      image: %s
      command: ["scaffold", "$(params.path)"]
`, DeployerImage)
}

// GetClusterTasks returns multi-document yaml containing tekton tasks used by func.
func GetClusterTasks() string {
	tasks := getBuildpackTask() + "\n---\n" + getS2ITask() + "\n---\n" + getDeployTask() + "\n---\n" + getScaffoldTask()
	tasks = strings.Replace(tasks, "kind: Task", "kind: ClusterTask", -1)
	tasks = strings.ReplaceAll(tasks, "apiVersion: tekton.dev/v1", "apiVersion: tekton.dev/v1beta1")
	return tasks
}
