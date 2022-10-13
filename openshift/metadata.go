package openshift

import (
	fn "knative.dev/func"
)

const (
	annotationOpenShiftVcsUri = "app.openshift.io/vcs-uri"
	annotationOpenShiftVcsRef = "app.openshift.io/vcs-ref"

	labelAppK8sInstance   = "app.kubernetes.io/instance"
	labelOpenShiftRuntime = "app.openshift.io/runtime"
)

var iconValuesForRuntimes = map[string]string{
	"go":         "golang",
	"node":       "nodejs",
	"python":     "python",
	"quarkus":    "quarkus",
	"springboot": "spring-boot",
}

type OpenshiftMetadataDecorator struct{}

func (o OpenshiftMetadataDecorator) UpdateAnnotations(f fn.Function, annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationOpenShiftVcsUri] = f.Build.Git.URL
	annotations[annotationOpenShiftVcsRef] = f.Build.Git.Revision

	return annotations
}

func (o OpenshiftMetadataDecorator) UpdateLabels(f fn.Function, labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}

	// this label is used for referencing a Tekton Pipeline and deployed KService
	labels[labelAppK8sInstance] = f.Name

	// if supported, set the label representing a runtime icon in Developer Console
	iconValue, ok := iconValuesForRuntimes[f.Runtime]
	if ok {
		labels[labelOpenShiftRuntime] = iconValue
	}

	return labels
}
