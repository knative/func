package openshift

import (
	fn "knative.dev/kn-plugin-func"
)

const (
	AnnotationOpenShiftVcsUri = "app.openshift.io/vcs-uri"
	AnnotationOpenShiftVcsRef = "app.openshift.io/vcs-ref"

	LabelAppK8sInstance = "app.kubernetes.io/instance"
)

type OpenshiftMetadataDecorator struct{}

func (o OpenshiftMetadataDecorator) UpdateAnnotations(f fn.Function, annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}
	if f.Git.URL != nil {
		annotations[AnnotationOpenShiftVcsUri] = *f.Git.URL
	}
	if f.Git.Revision != nil {
		annotations[AnnotationOpenShiftVcsRef] = *f.Git.Revision
	}

	return annotations
}

func (o OpenshiftMetadataDecorator) UpdateLabels(f fn.Function, labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}

	labels[LabelAppK8sInstance] = f.Name

	return labels
}
