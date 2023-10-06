package tekton

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	BashImage    = "docker.io/library/bash:5.1.4@sha256:b208215a4655538be652b2769d82e576bc4d0a2bb132144c060efc5be8c3f5d6"
	S2IImage     = "quay.io/boson/s2i:latest"
	FuncImage    = "ghcr.io/knative/func/func:latest"
	BuildahImage = "quay.io/buildah/stable:v1.31.0"
)

var BuildpackTask = v1beta1.Task{
	TypeMeta: metaV1.TypeMeta{
		Kind:       "Task",
		APIVersion: "tekton.dev/v1beta1",
	},
	ObjectMeta: metaV1.ObjectMeta{
		Name: "func-buildpacks",
		Labels: map[string]string{
			"app.kubernetes.io/version": "0.1",
		},
		Annotations: map[string]string{
			"tekton.dev/displayName":          "Knative Functions Buildpacks",
			"tekton.dev/pipelines.minVersion": "0.17.0",
			"tekton.dev/platforms":            "linux/amd64",
			"tekton.dev/tags":                 "image-build",
			"tekton.dev/categories":           "Image Build",
		},
	},
	Spec: v1beta1.TaskSpec{
		Params: []v1beta1.ParamSpec{
			v1beta1.ParamSpec{
				Name:        "APP_IMAGE",
				Description: "The name of where to store the app image.",
			},
			v1beta1.ParamSpec{
				Name:        "REGISTRY",
				Description: "The registry associated with the function image.",
			},
			v1beta1.ParamSpec{
				Name:        "BUILDER_IMAGE",
				Description: "The image on which builds will run (must include lifecycle and compatible buildpacks).",
			},
			v1beta1.ParamSpec{
				Name:        "SOURCE_SUBPATH",
				Description: "A subpath within the `source` input where the source to build is located.",
				Default: &v1beta1.ParamValue{
					Type: "string",
				},
			},
			v1beta1.ParamSpec{
				Name:        "ENV_VARS",
				Type:        "array",
				Description: "Environment variables to set during _build-time_.",
				Default: &v1beta1.ParamValue{
					Type:     "array",
					ArrayVal: []string{},
				},
			},
			v1beta1.ParamSpec{
				Name:        "RUN_IMAGE",
				Description: "Reference to a run image to use.",
				Default: &v1beta1.ParamValue{
					Type: "string",
				},
			},
			v1beta1.ParamSpec{
				Name:        "CACHE_IMAGE",
				Description: "The name of the persistent app cache image (if no cache workspace is provided).",
				Default: &v1beta1.ParamValue{
					Type: "string",
				},
			},
			v1beta1.ParamSpec{
				Name:        "SKIP_RESTORE",
				Description: "Do not write layer metadata or restore cached layers.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "false",
				},
			},
			v1beta1.ParamSpec{
				Name:        "USER_ID",
				Description: "The user ID of the builder image user.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "1001",
				},
			},
			v1beta1.ParamSpec{
				Name:        "GROUP_ID",
				Description: "The group ID of the builder image user.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "0",
				},
			},
			v1beta1.ParamSpec{
				Name:        "PLATFORM_DIR",
				Description: "The name of the platform directory.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "empty-dir",
				},
			},
		},
		Description: "The Knative Functions Buildpacks task builds source into a container image and pushes it to a registry, using Cloud Native Buildpacks. This task is based on the Buildpacks Tekton task v 0.4.",
		Steps: []v1beta1.Step{
			v1beta1.Step{
				Name:  "prepare",
				Image: BashImage,
				Args: []string{
					"--env-vars",
					"$(params.ENV_VARS[*])",
				},
				VolumeMounts: []v1.VolumeMount{
					v1.VolumeMount{
						Name:      "layers-dir",
						MountPath: "/layers",
					},
					v1.VolumeMount{
						Name:      "$(params.PLATFORM_DIR)",
						MountPath: "/platform",
					},
					v1.VolumeMount{
						Name:      "empty-dir",
						MountPath: "/emptyDir",
					},
				},
				Script: `#!/usr/bin/env bash
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
`,
			},
			v1beta1.Step{
				Name:  "create",
				Image: "$(params.BUILDER_IMAGE)",
				Command: []string{
					"/cnb/lifecycle/creator",
				},
				Args: []string{
					"-app=$(workspaces.source.path)/$(params.SOURCE_SUBPATH)",
					"-cache-dir=$(workspaces.cache.path)",
					"-cache-image=$(params.CACHE_IMAGE)",
					"-uid=$(params.USER_ID)",
					"-gid=$(params.GROUP_ID)",
					"-layers=/layers",
					"-platform=/platform",
					"-report=/layers/report.toml",
					"-skip-restore=$(params.SKIP_RESTORE)",
					"-previous-image=$(params.APP_IMAGE)",
					"-run-image=$(params.RUN_IMAGE)",
					"$(params.APP_IMAGE)",
				},
				Env: []v1.EnvVar{
					v1.EnvVar{
						Name:  "DOCKER_CONFIG",
						Value: "$(workspaces.dockerconfig.path)",
					},
				},
				VolumeMounts: []v1.VolumeMount{
					v1.VolumeMount{
						Name:      "layers-dir",
						MountPath: "/layers",
					},
					v1.VolumeMount{
						Name:      "$(params.PLATFORM_DIR)",
						MountPath: "/platform",
					},
				},
				ImagePullPolicy: "Always",
				SecurityContext: &v1.SecurityContext{
					RunAsUser:  ptr(int64(1001)),
					RunAsGroup: ptr(int64(0)),
				},
			},
			v1beta1.Step{
				Name:  "results",
				Image: BashImage,
				VolumeMounts: []v1.VolumeMount{
					v1.VolumeMount{
						Name:      "layers-dir",
						MountPath: "/layers",
					},
					v1.VolumeMount{
						Name:      "empty-dir",
						MountPath: "/emptyDir",
					},
				},
				Script: `#!/usr/bin/env bash
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
`,
			},
		},
		Volumes: []v1.Volume{
			v1.Volume{
				Name: "empty-dir",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
			v1.Volume{
				Name: "layers-dir",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
		},
		StepTemplate: &v1beta1.StepTemplate{
			Env: []v1.EnvVar{
				v1.EnvVar{
					Name:  "CNB_PLATFORM_API",
					Value: "0.10",
				},
			},
		},
		Workspaces: []v1beta1.WorkspaceDeclaration{
			v1beta1.WorkspaceDeclaration{
				Name:        "source",
				Description: "Directory where application source is located.",
			},
			v1beta1.WorkspaceDeclaration{
				Name:        "cache",
				Description: "Directory where cache is stored (when no cache image is provided).",
				Optional:    true,
			},
			v1beta1.WorkspaceDeclaration{
				Name:        "dockerconfig",
				Description: "An optional workspace that allows providing a .docker/config.json file for Buildpacks lifecycle binary to access the container registry. The file should be placed at the root of the Workspace with name config.json.",
				Optional:    true,
			},
		},
		Results: []v1beta1.TaskResult{
			v1beta1.TaskResult{
				Name:        "IMAGE_DIGEST",
				Description: "The digest of the built `APP_IMAGE`.",
			},
		},
	},
}

var S2ITask = v1beta1.Task{
	TypeMeta: metaV1.TypeMeta{
		Kind:       "Task",
		APIVersion: "tekton.dev/v1beta1",
	},
	ObjectMeta: metaV1.ObjectMeta{
		Name: "func-s2i",
		Labels: map[string]string{
			"app.kubernetes.io/version": "0.1",
		},
		Annotations: map[string]string{
			"tekton.dev/pipelines.minVersion": "0.17.0",
			"tekton.dev/platforms":            "linux/amd64",
			"tekton.dev/tags":                 "image-build",
			"tekton.dev/categories":           "Image Build",
		},
	},
	Spec: v1beta1.TaskSpec{
		Params: []v1beta1.ParamSpec{
			v1beta1.ParamSpec{
				Name:        "BUILDER_IMAGE",
				Description: "The location of the s2i builder image.",
			},
			v1beta1.ParamSpec{
				Name:        "APP_IMAGE",
				Description: "Reference of the image S2I will produce.",
			},
			v1beta1.ParamSpec{
				Name:        "REGISTRY",
				Description: "The registry associated with the function image.",
			},
			v1beta1.ParamSpec{
				Name:        "PATH_CONTEXT",
				Description: "The location of the path to run s2i from.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: ".",
				},
			},
			v1beta1.ParamSpec{
				Name:        "TLSVERIFY",
				Description: "Verify the TLS on the registry endpoint (for push/pull to a non-TLS registry)",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "true",
				},
			},
			v1beta1.ParamSpec{
				Name:        "LOGLEVEL",
				Description: "Log level when running the S2I binary",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "0",
				},
			},
			v1beta1.ParamSpec{
				Name:        "ENV_VARS",
				Type:        "array",
				Description: "Environment variables to set during _build-time_.",
				Default: &v1beta1.ParamValue{
					Type:     "array",
					ArrayVal: []string{},
				},
			},
			v1beta1.ParamSpec{
				Name:        "S2I_IMAGE_SCRIPTS_URL",
				Description: "The URL containing the default assemble and run scripts for the builder image.",
				Default: &v1beta1.ParamValue{
					Type:      "string",
					StringVal: "image:///usr/libexec/s2i",
				},
			},
		},
		Description: `Knative Functions Source-to-Image (S2I) is a toolkit and workflow for building reproducible container images from source code
S2I produces images by injecting source code into a base S2I container image and letting the container prepare that source code for execution. The base S2I container images contains the language runtime and build tools needed for building and running the source code.`,
		Steps: []v1beta1.Step{
			v1beta1.Step{
				Name:  "generate",
				Image: S2IImage,
				Args: []string{
					"$(params.ENV_VARS[*])",
				},
				WorkingDir: "$(workspaces.source.path)",
				VolumeMounts: []v1.VolumeMount{
					v1.VolumeMount{
						Name:      "gen-source",
						MountPath: "/gen-source",
					},
					v1.VolumeMount{
						Name:      "env-vars",
						MountPath: "/env-vars",
					},
				},
				Script: `echo "Processing Build Environment Variables"
echo "" > /env-vars/env-file
for var in "$@"
do
    if [[ "$var" != "=" ]]; then
        echo "$var" >> /env-vars/env-file
    fi
done

echo "Generated Build Env Var file"
echo "------------------------------"
cat /env-vars/env-file
echo "------------------------------"

/usr/local/bin/s2i --loglevel=$(params.LOGLEVEL) build $(params.PATH_CONTEXT) $(params.BUILDER_IMAGE) \
--image-scripts-url $(params.S2I_IMAGE_SCRIPTS_URL) \
--as-dockerfile /gen-source/Dockerfile.gen --environment-file /env-vars/env-file

echo "Preparing func.yaml for later deployment"
func_file="$(workspaces.source.path)/func.yaml"
if [ "$(params.PATH_CONTEXT)" != "" ]; then
  func_file="$(workspaces.source.path)/$(params.PATH_CONTEXT)/func.yaml"
fi
sed -i "s|^registry:.*$|registry: $(params.REGISTRY)|" "$func_file"
echo "Function image registry: $(params.REGISTRY)"

s2iignore_file="$(dirname "$func_file")/.s2iignore"
[ -f "$s2iignore_file" ] || echo "node_modules" >> "$s2iignore_file"
`,
			},
			v1beta1.Step{
				Name:       "build",
				Image:      BuildahImage,
				WorkingDir: "/gen-source",
				VolumeMounts: []v1.VolumeMount{
					v1.VolumeMount{
						Name:      "varlibcontainers",
						MountPath: "/var/lib/containers",
					},
					v1.VolumeMount{
						Name:      "gen-source",
						MountPath: "/gen-source",
					},
				},
				SecurityContext: &v1.SecurityContext{
					Capabilities: &v1.Capabilities{
						Add: []v1.Capability{
							"SETFCAP",
						},
					},
				},
				Script: `TLS_VERIFY_FLAG=""
if [ "$(params.TLSVERIFY)" = "false" ] || [ "$(params.TLSVERIFY)" = "0" ]; then
  TLS_VERIFY_FLAG="--tls-verify=false"
fi

[[ "$(workspaces.sslcertdir.bound)" == "true" ]] && CERT_DIR_FLAG="--cert-dir $(workspaces.sslcertdir.path)"
ARTIFACTS_CACHE_PATH="$(workspaces.cache.path)/mvn-artifacts"
[ -d "${ARTIFACTS_CACHE_PATH}" ] || mkdir "${ARTIFACTS_CACHE_PATH}"
buildah ${CERT_DIR_FLAG} bud --storage-driver=vfs ${TLS_VERIFY_FLAG} --layers \
  -v "${ARTIFACTS_CACHE_PATH}:/tmp/artifacts/:rw,z,U" \
  -f /gen-source/Dockerfile.gen -t $(params.APP_IMAGE) .

[[ "$(workspaces.dockerconfig.bound)" == "true" ]] && export DOCKER_CONFIG="$(workspaces.dockerconfig.path)"
buildah ${CERT_DIR_FLAG} push --storage-driver=vfs ${TLS_VERIFY_FLAG} --digestfile $(workspaces.source.path)/image-digest \
  $(params.APP_IMAGE) docker://$(params.APP_IMAGE)

cat $(workspaces.source.path)/image-digest | tee /tekton/results/IMAGE_DIGEST
`,
			},
		},
		Volumes: []v1.Volume{
			v1.Volume{
				Name: "varlibcontainers",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
			v1.Volume{
				Name: "gen-source",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
			v1.Volume{
				Name: "env-vars",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
		},
		Workspaces: []v1beta1.WorkspaceDeclaration{
			v1beta1.WorkspaceDeclaration{
				Name: "source",
			},
			v1beta1.WorkspaceDeclaration{
				Name:        "cache",
				Description: "Directory where cache is stored (e.g. local mvn repo).",
				Optional:    true,
			},
			v1beta1.WorkspaceDeclaration{
				Name:     "sslcertdir",
				Optional: true,
			},
			v1beta1.WorkspaceDeclaration{
				Name:        "dockerconfig",
				Description: "An optional workspace that allows providing a .docker/config.json file for Buildah to access the container registry. The file should be placed at the root of the Workspace with name config.json.",
				Optional:    true,
			},
		},
		Results: []v1beta1.TaskResult{
			v1beta1.TaskResult{
				Name:        "IMAGE_DIGEST",
				Description: "Digest of the image just built.",
			},
		},
	},
}

var DeployTask = v1beta1.Task{
	TypeMeta: metaV1.TypeMeta{
		Kind:       "Task",
		APIVersion: "tekton.dev/v1beta1",
	},
	ObjectMeta: metaV1.ObjectMeta{
		Name: "func-deploy",
		Labels: map[string]string{
			"app.kubernetes.io/version": "0.1",
		},
		Annotations: map[string]string{
			"tekton.dev/pipelines.minVersion": "0.12.1",
			"tekton.dev/platforms":            "linux/amd64",
			"tekton.dev/tags":                 "cli",
			"tekton.dev/categories":           "CLI",
		},
	},
	Spec: v1beta1.TaskSpec{
		Params: []v1beta1.ParamSpec{
			v1beta1.ParamSpec{
				Name:        "path",
				Description: "Path to the function project",
				Default: &v1beta1.ParamValue{
					Type: "string",
				},
			},
			v1beta1.ParamSpec{
				Name:        "image",
				Description: "Container image to be deployed",
				Default: &v1beta1.ParamValue{
					Type: "string",
				},
			},
		},
		Description: "This Task performs a deploy operation using the Knative `func` CLI",
		Steps: []v1beta1.Step{
			v1beta1.Step{
				Name:  "func-deploy",
				Image: FuncImage,
				Script: `func deploy --verbose --build=false --push=false --path=$(params.path) --remote=false --image="$(params.image)"
`,
			},
		},
		Workspaces: []v1beta1.WorkspaceDeclaration{
			v1beta1.WorkspaceDeclaration{
				Name:        "source",
				Description: "The workspace containing the function project",
			},
		},
	},
}

func ptr[T any](val T) *T {
	return &val
}
