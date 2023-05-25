package tekton

import (
	"fmt"

	pplnv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

const (
	taskNameFetchSources = "fetch-sources"
	taskNameBuild        = "build"
	taskNameDeploy       = "deploy"
)

func taskFetchSources() pplnv1beta1.PipelineTask {
	return pplnv1beta1.PipelineTask{
		Name: taskNameFetchSources,
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "git-clone",
		},
		Workspaces: []pplnv1beta1.WorkspacePipelineTaskBinding{{
			Name:      "output",
			Workspace: "source-workspace",
		}},
		Params: []pplnv1beta1.Param{
			{Name: "url", Value: *pplnv1beta1.NewArrayOrString("$(params.gitRepository)")},
			{Name: "revision", Value: *pplnv1beta1.NewArrayOrString("$(params.gitRevision)")},
		},
	}
}

func taskBuildpacks(runAfter []string) pplnv1beta1.PipelineTask {
	return pplnv1beta1.PipelineTask{
		Name: taskNameBuild,
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "func-buildpacks",
		},
		RunAfter: runAfter,
		Workspaces: []pplnv1beta1.WorkspacePipelineTaskBinding{
			{
				Name:      "source",
				Workspace: "source-workspace",
			},
			{
				Name:      "cache",
				Workspace: "cache-workspace",
			},
			{
				Name:      "dockerconfig",
				Workspace: "dockerconfig-workspace",
			}},
		Params: []pplnv1beta1.Param{
			{Name: "APP_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.imageName)")},
			{Name: "REGISTRY", Value: *pplnv1beta1.NewArrayOrString("$(params.registry)")},
			{Name: "SOURCE_SUBPATH", Value: *pplnv1beta1.NewArrayOrString("$(params.contextDir)")},
			{Name: "BUILDER_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.builderImage)")},
			{Name: "ENV_VARS", Value: pplnv1beta1.ArrayOrString{
				Type:     pplnv1beta1.ParamTypeArray,
				ArrayVal: []string{"$(params.buildEnvs[*])"},
			}},
		},
	}

}
func taskS2iBuild(runAfter []string) pplnv1beta1.PipelineTask {
	params := []pplnv1beta1.Param{
		{Name: "IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.imageName)")},
		{Name: "REGISTRY", Value: *pplnv1beta1.NewArrayOrString("$(params.registry)")},
		{Name: "PATH_CONTEXT", Value: *pplnv1beta1.NewArrayOrString("$(params.contextDir)")},
		{Name: "BUILDER_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.builderImage)")},
		{Name: "ENV_VARS", Value: pplnv1beta1.ArrayOrString{
			Type:     pplnv1beta1.ParamTypeArray,
			ArrayVal: []string{"$(params.buildEnvs[*])"},
		}},
		{Name: "S2I_IMAGE_SCRIPTS_URL", Value: *pplnv1beta1.NewArrayOrString("$(params.s2iImageScriptsUrl)")},
	}
	return pplnv1beta1.PipelineTask{
		Name: taskNameBuild,
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "func-s2i",
		},
		RunAfter: runAfter,
		Workspaces: []pplnv1beta1.WorkspacePipelineTaskBinding{
			{
				Name:      "source",
				Workspace: "source-workspace",
			},
			{
				Name:      "cache",
				Workspace: "cache-workspace",
			},
			{
				Name:      "dockerconfig",
				Workspace: "dockerconfig-workspace",
			}},
		Params: params,
	}

}

func taskDeploy(runAfter string) pplnv1beta1.PipelineTask {

	params := []pplnv1beta1.Param{{Name: "path", Value: *pplnv1beta1.NewArrayOrString("$(workspaces.source.path)/$(params.contextDir)")}}

	// Deploy step that uses an image produced by builds needs explicit reference to the image
	params = append(params, pplnv1beta1.Param{Name: "image", Value: *pplnv1beta1.NewArrayOrString(fmt.Sprintf("$(params.imageName)@$(tasks.%s.results.IMAGE_DIGEST)", runAfter))})

	return pplnv1beta1.PipelineTask{
		Name: taskNameDeploy,
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "func-deploy",
		},
		RunAfter: []string{runAfter},
		Workspaces: []pplnv1beta1.WorkspacePipelineTaskBinding{{
			Name:      "source",
			Workspace: "source-workspace",
		}},
		Params: params,
	}
}
