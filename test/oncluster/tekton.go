package oncluster

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/pipelines/tekton"
)

// TektonPipelineExists verifies pipeline with a given function name exists on cluster
func TektonPipelineExists(t *testing.T, functionName string) bool {
	ns, _, _ := k8s.GetClientConfig().Namespace()
	client, _ := tekton.NewTektonClient(ns)
	// Look for pipelines with the function.knative.dev/name label matching the function name
	pipelines, err := client.Pipelines(ns).List(context.Background(), v1.ListOptions{
		LabelSelector: "function.knative.dev/name=" + functionName,
	})
	if err != nil {
		t.Error(err.Error())
		return false
	}
	// If any pipeline exists with this label, it means the pipeline was created for this function
	return len(pipelines.Items) > 0
}

// TektonPipelineRunExists verifies pipelinerun with a given function name exists on cluster
func TektonPipelineRunExists(t *testing.T, functionName string) bool {
	ns, _, _ := k8s.GetClientConfig().Namespace()
	client, _ := tekton.NewTektonClient(ns)
	// Look for pipeline runs with the function.knative.dev/name label matching the function name
	pipelineRuns, err := client.PipelineRuns(ns).List(context.Background(), v1.ListOptions{
		LabelSelector: "function.knative.dev/name=" + functionName,
	})
	if err != nil {
		t.Error(err.Error())
		return false
	}
	// If any pipeline run exists with this label, it means a pipeline run was created for this function
	return len(pipelineRuns.Items) > 0
}

type PipelineRunSummary struct {
	PipelineRunName   string
	PipelineRunStatus string
	TasksRunSummary   []PipelineTaskRunSummary
}
type PipelineTaskRunSummary struct {
	TaskName   string
	TaskStatus string
}

func (p *PipelineRunSummary) ToString() string {
	r := fmt.Sprintf("run: %-42v, status: %v\n", p.PipelineRunName, p.PipelineRunStatus)
	for _, t := range p.TasksRunSummary {
		r = r + fmt.Sprintf(" task: %-15v, status: %v\n", t.TaskName, t.TaskStatus)
	}
	return r
}

func (p *PipelineRunSummary) IsSucceed() bool {
	return p.PipelineRunStatus == "Succeeded"
}

// TektonPipelineLastRunSummary gather information about a pipeline run such as
// list of tasks executed and status of each task execution. It is meant to be used on assertions
func TektonPipelineLastRunSummary(t *testing.T, functionName string) *PipelineRunSummary {
	ns, _, _ := k8s.GetClientConfig().Namespace()
	client, _ := tekton.NewTektonClient(ns)
	// Look for pipeline runs with the function.knative.dev/name label matching the function name
	pipelineRuns, err := client.PipelineRuns(ns).List(context.Background(), v1.ListOptions{
		LabelSelector: "function.knative.dev/name=" + functionName,
	})
	if err != nil {
		t.Error(err.Error())
		return &PipelineRunSummary{}
	}

	lr := PipelineRunSummary{}
	// Get the most recent pipeline run (they're sorted by creation time)
	if len(pipelineRuns.Items) > 0 {
		// Take the first one (most recent) from the filtered list
		run := pipelineRuns.Items[0]
		lr.PipelineRunName = run.Name
		if len(run.Status.Conditions) > 0 {
			lr.PipelineRunStatus = run.Status.Conditions[0].Reason
		}
		lr.TasksRunSummary = []PipelineTaskRunSummary{}
		for _, ref := range run.Status.ChildReferences {
			r := PipelineTaskRunSummary{}
			r.TaskName = ref.PipelineTaskName
			taskRun, err := client.TaskRuns(ns).Get(context.Background(), ref.Name, v1.GetOptions{})
			if err != nil {
				t.Error(err.Error())
			}
			if len(taskRun.Status.Conditions) > 0 {
				r.TaskStatus = taskRun.Status.Conditions[0].Reason
			}
			lr.TasksRunSummary = append(lr.TasksRunSummary, r)
		}
	}
	return &lr
}
