package tekton

import (
	"context"
	"fmt"

	pplnv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/buildpacks"
	"knative.dev/func/s2i"
)

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

func generatePipeline(f fn.Function, labels map[string]string) *pplnv1beta1.Pipeline {

	// -----  General properties
	pipelineName := getPipelineName(f)

	params := []pplnv1beta1.ParamSpec{
		{
			Name:        "gitRepository",
			Description: "Git repository that hosts the function project",
			Default:     pplnv1beta1.NewArrayOrString(f.Build.Git.URL),
		},
		{
			Name:        "gitRevision",
			Description: "Git revision to build",
		},
		{
			Name:        "contextDir",
			Description: "Path where the function project is",
			Default:     pplnv1beta1.NewArrayOrString(""),
		},
		{
			Name:        "imageName",
			Description: "Function image name",
		},
		{
			Name:        "builderImage",
			Description: "Builder image to be used",
		},
		{
			Name:        "buildEnvs",
			Description: "Environment variables to set during build time",
			Type:        "array",
		},
	}

	workspaces := []pplnv1beta1.PipelineWorkspaceDeclaration{
		{Name: "source-workspace", Description: "Directory where function source is located."},
		{Name: "dockerconfig-workspace", Description: "Directory containing image registry credentials stored in `config.json` file.", Optional: true},
	}

	var taskBuild pplnv1beta1.PipelineTask

	// Deploy step that uses an image produced by S2I builds needs explicit reference to the image
	referenceImageFromPreviousTaskResults := false

	var tasks []pplnv1beta1.PipelineTask
	var buildPreReq []string

	if f.Build.Git.URL != "" {
		// If Git is set up create fetch task,
		// otherwise sources have been already uploaded to workspace PVC.
		buildPreReq = []string{taskNameFetchSources}
		tasks = append(tasks, taskFetchSources())
	}

	if f.Build.Builder == builders.Pack {
		// ----- Buildpacks related properties
		workspaces = append(workspaces, pplnv1beta1.PipelineWorkspaceDeclaration{Name: "cache-workspace", Description: "Directory where Buildpacks cache is stored."})
		taskBuild = taskBuildpacks(buildPreReq)

	} else if f.Build.Builder == builders.S2I {
		// ----- S2I build related properties

		params = append(params, pplnv1beta1.ParamSpec{Name: "s2iImageScriptsUrl", Description: "URL containing the default assemble and run scripts for the builder image.",
			Default: pplnv1beta1.NewArrayOrString("image:///usr/libexec/s2i")})

		taskBuild = taskS2iBuild(buildPreReq)
		referenceImageFromPreviousTaskResults = true
	}

	// ----- Pipeline definition
	tasks = append(tasks, taskBuild, taskDeploy(taskNameBuild, referenceImageFromPreviousTaskResults))

	return &pplnv1beta1.Pipeline{
		ObjectMeta: v1.ObjectMeta{
			Name:        pipelineName,
			Labels:      labels,
			Annotations: f.Deploy.Annotations,
		},
		Spec: pplnv1beta1.PipelineSpec{
			Params:     params,
			Workspaces: workspaces,
			Tasks:      tasks,
		},
	}
}

func generatePipelineRun(f fn.Function, labels map[string]string) *pplnv1beta1.PipelineRun {

	revision := f.Build.Git.Revision
	contextDir := f.Build.Git.ContextDir
	if contextDir == "" && f.Build.Builder == builders.S2I {
		// TODO(lkingland): could instead update S2I to interpret empty string
		// as cwd, such that builder-specific code can be kept out of here.
		contextDir = "."
	}

	buildEnvs := &pplnv1beta1.ArrayOrString{
		Type:     pplnv1beta1.ParamTypeArray,
		ArrayVal: []string{},
	}
	if len(f.Build.BuildEnvs) > 0 {
		var envs []string
		for _, e := range f.Build.BuildEnvs {
			envs = append(envs, e.KeyValuePair())
		}
		buildEnvs.ArrayVal = envs
	} else {
		// need to hack empty BuildEnvs array on Tekton v0.39.0+
		// until https://github.com/tektoncd/pipeline/issues/5149 is resolved and released
		buildEnvs.ArrayVal = append(buildEnvs.ArrayVal, "=")
	}

	params := []pplnv1beta1.Param{
		{
			Name:  "gitRepository",
			Value: *pplnv1beta1.NewArrayOrString(f.Build.Git.URL),
		},
		{
			Name:  "gitRevision",
			Value: *pplnv1beta1.NewArrayOrString(revision),
		},
		{
			Name:  "contextDir",
			Value: *pplnv1beta1.NewArrayOrString(contextDir),
		},
		{
			Name:  "imageName",
			Value: *pplnv1beta1.NewArrayOrString(f.Image),
		},
		{
			Name:  "builderImage",
			Value: *pplnv1beta1.NewArrayOrString(getBuilderImage(f)),
		},
		{
			Name:  "buildEnvs",
			Value: *buildEnvs,
		},
	}

	workspaces := []pplnv1beta1.WorkspaceBinding{
		{
			Name: "source-workspace",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: getPipelinePvcName(f),
			},
			SubPath: "source",
		},
		{
			Name: "dockerconfig-workspace",
			Secret: &corev1.SecretVolumeSource{
				SecretName: getPipelineSecretName(f),
			},
		},
	}

	if f.Build.Builder == builders.Pack {
		// ----- Buildpacks related properties

		workspaces = append(workspaces, pplnv1beta1.WorkspaceBinding{
			Name: "cache-workspace",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: getPipelinePvcName(f),
			},
			SubPath: "cache",
		})
	} else if f.Build.Builder == builders.S2I {
		if f.Runtime == "quarkus" {
			params = append(params, pplnv1beta1.Param{Name: "s2iImageScriptsUrl", Value: *pplnv1beta1.NewArrayOrString("image:///usr/local/s2i")})
		}
	}

	// ----- PipelineRun definition
	return &pplnv1beta1.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-run-", getPipelineName(f)),
			Labels:       labels,
			Annotations:  f.Deploy.Annotations,
		},
		Spec: pplnv1beta1.PipelineRunSpec{
			PipelineRef: &pplnv1beta1.PipelineRef{
				Name: getPipelineName(f),
			},
			Params:     params,
			Workspaces: workspaces,
		},
	}
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

func getPipelineSecretName(f fn.Function) string {
	return fmt.Sprintf("%s-secret", getPipelineName(f))
}

func getPipelinePvcName(f fn.Function) string {
	return fmt.Sprintf("%s-pvc", getPipelineName(f))
}
