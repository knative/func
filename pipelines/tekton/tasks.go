package tekton

import (
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

func taskBuild(runAfter string) pplnv1beta1.PipelineTask {
	return pplnv1beta1.PipelineTask{
		Name: taskNameBuild,
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "func-buildpacks",
		},
		RunAfter: []string{runAfter},
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
			{Name: "SOURCE_SUBPATH", Value: *pplnv1beta1.NewArrayOrString("$(params.contextDir)")},
			{Name: "BUILDER_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.builderImage)")},
		},
	}
}

func taskDeploy(runAfter string) pplnv1beta1.PipelineTask {
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
		Params: []pplnv1beta1.Param{
			{Name: "path", Value: *pplnv1beta1.NewArrayOrString("$(workspaces.source.path)/$(params.contextDir)")},
		},
	}
}
