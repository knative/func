package formatting

import (
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
)

// PipelineRunStatus return status of PR  success failed or skipped
func PipelineRunStatus(pr *tektonv1.PipelineRun) string {
	if len(pr.Status.Conditions) == 0 {
		return "neutral"
	}
	if pr.Status.Conditions[0].Status == corev1.ConditionFalse {
		return "failure"
	}
	return "success"
}
