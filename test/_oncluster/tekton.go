package oncluster

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/kn-plugin-func/k8s"
	"knative.dev/kn-plugin-func/pipelines/tekton"
)

// TektonPipelineExists verifies pipeline with a given prefix exists on cluster
func TektonPipelineExists(t *testing.T, pipelinePrefix string) bool {
	namespace, _, _ := k8s.GetClientConfig().Namespace()
	client, ns, _ := tekton.NewTektonClientAndResolvedNamespace(namespace)
	pipelines, err := client.Pipelines(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		t.Error(err.Error())
	}
	for _, pipeline := range pipelines.Items {
		if strings.HasPrefix(pipeline.Name, pipelinePrefix) && strings.HasSuffix(pipeline.Name, "-pipeline") {
			return true
		}
	}
	return false
}

// TektonPipelineRunExists verifies pipelinerun with a given prefix exists on cluster
func TektonPipelineRunExists(t *testing.T, pipelineRunPrefix string) bool {
	namespace, _, _ := k8s.GetClientConfig().Namespace()
	client, ns, _ := tekton.NewTektonClientAndResolvedNamespace(namespace)
	pipelineRuns, err := client.PipelineRuns(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		t.Error(err.Error())
	}
	for _, run := range pipelineRuns.Items {
		if strings.HasPrefix(run.Name, pipelineRunPrefix) {
			return true
		}
	}
	return false
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

// TektonPipelTektonPipelineLastRunSummary gather information about a pipeline run such as
// list of tasks executed and status of each task execution. It is meant to be used on assertions
func TektonPipelineLastRunSummary(t *testing.T, pipelinePrefix string) *PipelineRunSummary {
	namespace, _, _ := k8s.GetClientConfig().Namespace()
	client, ns, _ := tekton.NewTektonClientAndResolvedNamespace(namespace)
	pipelineRuns, err := client.PipelineRuns(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		t.Error(err.Error())
	}
	lr := PipelineRunSummary{}
	for _, run := range pipelineRuns.Items {
		if strings.HasPrefix(run.Name, pipelinePrefix) {
			lr.PipelineRunName = run.Name
			if len(run.Status.Conditions) > 0 {
				lr.PipelineRunStatus = run.Status.Conditions[0].Reason
			}
			lr.TasksRunSummary = []PipelineTaskRunSummary{}
			for _, taskRun := range run.Status.TaskRuns {
				trun := PipelineTaskRunSummary{}
				trun.TaskName = taskRun.PipelineTaskName
				if len(taskRun.Status.Conditions) > 0 {
					trun.TaskStatus = taskRun.Status.Conditions[0].Reason
				}
				lr.TasksRunSummary = append(lr.TasksRunSummary, trun)
			}
		}
	}
	return &lr
}
