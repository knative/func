//go:build !integration
// +build !integration

package tekton

import (
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
)

func Test_generatePipeline(t *testing.T) {
	testGitRepo := "http://git-repo/git.git"
	testGit := fn.Git{
		URL: &testGitRepo,
	}

	tests := []struct {
		name          string
		function      fn.Function
		taskBuildName string
	}{
		{
			name:          "Pack builder - use buildpacks",
			function:      fn.Function{Builder: builders.Pack, Git: testGit},
			taskBuildName: "func-buildpacks",
		},
		{
			name:          "s2i builder - use",
			function:      fn.Function{Builder: builders.S2I, Git: testGit},
			taskBuildName: "func-s2i",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ppl := generatePipeline(tt.function, map[string]string{})

			for _, task := range ppl.Spec.Tasks {
				// let's check what is the Task used for build task
				if task.Name == taskNameBuild {
					if task.TaskRef.Name != tt.taskBuildName {
						t.Errorf("generatePipeline(), for builder = %q: wanted build Task = %q, got = %q", tt.function.Builder, tt.taskBuildName, task.TaskRef.Name)
					}
					return
				}
			}

			// we haven't found the build Task -> fail
			t.Errorf("generatePipeline(), wasn't able to find build related task named = %q", taskNameBuild)
		})
	}
}
