package tekton

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

var (
	FuncUtilImage = "ghcr.io/knative/func-utils:v2"
	DeployerImage string
	ScaffoldImage string
	S2IImage      string
	//go:embed task-buildpack.yaml.tmpl
	buildpackTaskTemplate string
	//go:embed task-s2i.yaml.tmpl
	s2iTaskTemplate string
)

func init() {
	if DeployerImage == "" {
		DeployerImage = FuncUtilImage
	}
	if ScaffoldImage == "" {
		ScaffoldImage = FuncUtilImage
	}
	if S2IImage == "" {
		S2IImage = FuncUtilImage
	}
}

func getBuildpackTask() string {
	return getTask(buildpackTaskTemplate)
}

func getS2ITask() string {
	return getTask(s2iTaskTemplate)
}

// GetClusterTasks returns multi-document yaml containing tekton tasks used by func.
func GetClusterTasks() string {
	tasks := getBuildpackTask() + "\n---\n" + getS2ITask()
	tasks = strings.ReplaceAll(tasks, "kind: Task", "kind: ClusterTask")
	tasks = strings.ReplaceAll(tasks, "apiVersion: tekton.dev/v1", "apiVersion: tekton.dev/v1beta1")
	return tasks
}

func getTask(t string) string {
	tmpl, err := template.New("").Parse(t)
	if err != nil {
		panic(err)
	}
	var buff bytes.Buffer
	err = tmpl.Execute(&buff, &struct {
		DeployerImage, ScaffoldImage, S2IImage string
	}{DeployerImage, ScaffoldImage, S2IImage})
	if err != nil {
		panic(err)
	}
	return buff.String()
}
