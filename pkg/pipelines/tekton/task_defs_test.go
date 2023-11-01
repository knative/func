package tekton_test

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"

	"knative.dev/func/pkg/pipelines/tekton"
)

func TestTaskMatch(t *testing.T) {
	for _, tt := range []struct {
		path string
		task v1beta1.Task
	}{
		{
			path: "../resources/tekton/task/func-buildpacks/0.2/func-buildpacks.yaml",
			task: tekton.BuildpackTask,
		},
		{
			path: "../resources/tekton/task/func-s2i/0.2/func-s2i.yaml",
			task: tekton.S2ITask,
		},
		{
			path: "../resources/tekton/task/func-deploy/0.1/func-deploy.yaml",
			task: tekton.DeployTask,
		},
	} {
		t.Run(tt.task.Name, func(t *testing.T) {

			f, err := os.Open(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			dec := k8sYaml.NewYAMLToJSONDecoder(f)
			var taskFromYaml v1beta1.Task
			err = dec.Decode(&taskFromYaml)
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(tt.task, taskFromYaml); d != "" {
				t.Error("output missmatch (-want, +got):", d)
			}
		})
	}
}
