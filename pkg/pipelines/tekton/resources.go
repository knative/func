package tekton

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
)

func getPipeline(f fn.Function, labels map[string]string) (*v1beta1.Pipeline, error) {
	pipelineFromFile, err := loadResource[*v1beta1.Pipeline](path.Join(f.Root, resourcesDirectory, pipelineFileName))
	if err != nil {
		return nil, fmt.Errorf("cannot load resource from file: %v", err)
	}
	if pipelineFromFile != nil {
		name := getPipelineName(f)
		if pipelineFromFile.Name != name {
			return nil, fmt.Errorf("resource name missmatch: %q != %q", pipelineFromFile.Name, name)
		}
		return pipelineFromFile, nil
	}

	var buildTaskSpec v1beta1.TaskSpec
	switch f.Build.Builder {
	case builders.S2I:
		buildTaskSpec = *S2ITask.Spec.DeepCopy()
	case builders.Pack:
		buildTaskSpec = *BuildpackTask.Spec.DeepCopy()
	default:
		return nil, fmt.Errorf("unsupported builder: %q", f.Build.BuilderImages)
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
				TaskSpec: buildTaskSpec,
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
					Name: "SOURCE_SUBPATH",
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

func getPipelineRun(f fn.Function, labels map[string]string) (*v1beta1.PipelineRun, error) {
	pipelineRunFromFile, err := loadResource[*v1beta1.PipelineRun](path.Join(f.Root, resourcesDirectory, pipelineRunFilenane))
	if err != nil {
		return nil, fmt.Errorf("cannot load resource from file: %v", err)
	}
	if pipelineRunFromFile != nil {
		generateName := getPipelineRunGenerateName(f)
		if pipelineRunFromFile.GetGenerateName() != generateName {
			return nil, fmt.Errorf("resource name missmatch: %q != %q", pipelineRunFromFile.GetGenerateName(), generateName)
		}
		return pipelineRunFromFile, nil
	}

	if labels == nil {
		labels = make(map[string]string, 1)
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

type res interface {
	GetGroupVersionKind() schema.GroupVersionKind
	GetObjectKind() schema.ObjectKind
}

func loadResource[T res](fileName string) (T, error) {
	var result T
	filePath := fileName
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		var file *os.File
		file, err = os.Open(filePath)
		if err != nil {
			return result, fmt.Errorf("cannot opern resource file: %w", err)
		}
		defer file.Close()
		dec := k8sYaml.NewYAMLToJSONDecoder(file)
		err = dec.Decode(&result)
		if err != nil {
			return result, fmt.Errorf("cannot deserialize resource: %w", err)
		}
		gvk := result.GetGroupVersionKind()
		if gvk != result.GetObjectKind().GroupVersionKind() {
			return result, fmt.Errorf("unexpected resource type: %q", result.GetObjectKind().GroupVersionKind())
		}
		return result, nil
	}
	return result, nil
}

func deletePipelines(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewTektonClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.Pipelines(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

func deletePipelineRuns(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewTektonClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.PipelineRuns(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

// guilderImage returns the builder image to use when building the Function
// with the Pack strategy if it can be calculated (the Function has a defined
// language runtime.  Errors are checked elsewhere, so at this level they
// manifest as an inability to get a builder image = empty string.
func getBuilderImage(f fn.Function) (name string) {
	if f.Build.Builder == builders.S2I {
		name, _ = s2i.BuilderImage(f, builders.S2I)
	} else {
		name, _ = buildpacks.BuilderImage(f, builders.Pack)
	}
	return
}

func getPipelineName(f fn.Function) string {
	var source string
	if f.Build.Git.URL == "" {
		source = "upload"
	} else {
		source = "git"
	}
	return fmt.Sprintf("%s-%s-%s-pipeline", f.Name, f.Build.Builder, source)
}

func getPipelineRunGenerateName(f fn.Function) string {
	return fmt.Sprintf("%s-run-", getPipelineName(f))
}

func getPipelineSecretName(f fn.Function) string {
	return fmt.Sprintf("%s-secret", getPipelineName(f))
}

func getPipelinePvcName(f fn.Function) string {
	return fmt.Sprintf("%s-pvc", getPipelineName(f))
}
