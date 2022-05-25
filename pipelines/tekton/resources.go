package tekton

import (
	"context"
	"fmt"

	pplnv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
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
	pipelineName := getPipelineName(f)

	params := []pplnv1beta1.ParamSpec{
		{
			Name:        "gitRepository",
			Description: "Git repository that hosts the function project",
			Default:     pplnv1beta1.NewArrayOrString(*f.Git.URL),
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
			Description: "Buildpacks builder image to be used",
		},
	}

	workspaces := []pplnv1beta1.PipelineWorkspaceDeclaration{
		{Name: "source-workspace", Description: "Directory where function source is located."},
		{Name: "cache-workspace", Description: "Directory where Buildpacks cache is stored."},
		{Name: "dockerconfig-workspace", Description: "Directory containing image registry credentials stored in `config.json` file.", Optional: true},
	}

	tasks := pplnv1beta1.PipelineTaskList{
		taskFetchSources(),
		taskBuild(taskNameFetchSources),
		taskDeploy(taskNameBuild),
	}

	return &pplnv1beta1.Pipeline{
		ObjectMeta: v1.ObjectMeta{
			Name:   pipelineName,
			Labels: labels,
		},
		Spec: pplnv1beta1.PipelineSpec{
			Params:     params,
			Workspaces: workspaces,
			Tasks:      tasks,
		},
	}
}

func generatePipelineRun(f fn.Function, labels map[string]string) *pplnv1beta1.PipelineRun {

	revision := ""
	if f.Git.Revision != nil {
		revision = *f.Git.Revision
	}
	contextDir := ""
	if f.Git.ContextDir != nil {
		contextDir = *f.Git.ContextDir
	}

	return &pplnv1beta1.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-run-", getPipelineName(f)),
			Labels:       labels,
		},

		Spec: pplnv1beta1.PipelineRunSpec{
			PipelineRef: &pplnv1beta1.PipelineRef{
				Name: getPipelineName(f),
			},

			Params: []pplnv1beta1.Param{
				{
					Name:  "gitRepository",
					Value: *pplnv1beta1.NewArrayOrString(*f.Git.URL),
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
			},

			Workspaces: []pplnv1beta1.WorkspaceBinding{
				{
					Name: "source-workspace",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: getPipelinePvcName(f),
					},
					SubPath: "source",
				},
				{
					Name: "cache-workspace",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: getPipelinePvcName(f),
					},
					SubPath: "cache",
				},
				{
					Name: "dockerconfig-workspace",
					Secret: &corev1.SecretVolumeSource{
						SecretName: getPipelineSecretName(f),
					},
				},
			},
		},
	}
}

// guilderImage returns the builder image to use when building the Function
// with the Pack strategy if it can be calculated (the Function has a defined
// language runtime.  Errors are checked elsewhere, so at this level they
// manifest as an inability to get a builder image = empty string.
func getBuilderImage(f fn.Function) (name string) {
	name, _ = buildpacks.BuilderImage(f)
	return
}

func getPipelineName(f fn.Function) string {
	return fmt.Sprintf("%s-%s-pipeline", f.Name, f.BuildType)
}

func getPipelineSecretName(f fn.Function) string {
	return fmt.Sprintf("%s-secret", getPipelineName(f))
}

func getPipelinePvcName(f fn.Function) string {
	return fmt.Sprintf("%s-pvc", getPipelineName(f))
}
