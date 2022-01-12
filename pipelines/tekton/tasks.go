package tekton

import (
	pplnv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func taskFetchRepository() pplnv1beta1.PipelineTask {
	return pplnv1beta1.PipelineTask{
		Name: "fetch-repository",
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
		Name: "build",
		TaskRef: &pplnv1beta1.TaskRef{
			Name: "buildpacks",
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
			}},
		Params: []pplnv1beta1.Param{
			{Name: "APP_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.imageName)")},
			{Name: "SOURCE_SUBPATH", Value: *pplnv1beta1.NewArrayOrString("$(params.contextDir)")},
			{Name: "BUILDER_IMAGE", Value: *pplnv1beta1.NewArrayOrString("$(params.builderImage)")},
		},
	}
}

// TODO this should be part of the future func-build Tekton Task as a post-build step
func taskImageDigest(runAfter string) pplnv1beta1.PipelineTask {
	script := `#!/usr/bin/env bash
set -e

func_file="/workspace/source/func.yaml"
if [ "$(params.contextDir)" != "" ]; then
  func_file="/workspace/source/$(params.contextDir)/func.yaml"
fi

sed -i "s|^image:.*$|image: $(params.image)|" "$func_file"
echo "Function image name: $(params.image)"

sed -i "s/^imageDigest:.*$/imageDigest: $(params.digest)/" "$func_file"
echo "Function image digest: $(params.digest)"
	`

	return pplnv1beta1.PipelineTask{
		Name: "image-digest",
		TaskSpec: &pplnv1beta1.EmbeddedTask{
			TaskSpec: pplnv1beta1.TaskSpec{
				Workspaces: []pplnv1beta1.WorkspaceDeclaration{
					{Name: "source"},
				},
				Steps: []pplnv1beta1.Step{
					{
						Container: corev1.Container{
							Image: "docker.io/library/bash:5.1.4@sha256:b208215a4655538be652b2769d82e576bc4d0a2bb132144c060efc5be8c3f5d6",
						},
						Script: script,
					},
				},
				Params: []pplnv1beta1.ParamSpec{
					{Name: "image"},
					{Name: "digest"},
					{Name: "contextDir"},
				},
			},
		},
		RunAfter: []string{runAfter},
		Workspaces: []pplnv1beta1.WorkspacePipelineTaskBinding{{
			Name:      "source",
			Workspace: "source-workspace",
		}},
		Params: []pplnv1beta1.Param{
			{Name: "image", Value: *pplnv1beta1.NewArrayOrString("$(params.imageName)")},
			{Name: "digest", Value: *pplnv1beta1.NewArrayOrString("$(tasks.build.results.APP_IMAGE_DIGEST)")},
			{Name: "contextDir", Value: *pplnv1beta1.NewArrayOrString("$(params.contextDir)")},
		},
	}
}

func taskFuncDeploy(runAfter string) pplnv1beta1.PipelineTask {
	return pplnv1beta1.PipelineTask{
		Name: "deploy",
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
