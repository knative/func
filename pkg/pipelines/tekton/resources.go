package tekton

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
)

func deletePipelines(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewTektonClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.Pipelines(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

func deletePipelineRuns(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewTektonClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.PipelineRuns(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

// guilderImage returns the builder image to use when building the Function
// with the Pack strategy if it can be calculated (the Function has a defined
// language runtime.  Errors are checked elsewhere, so at this level they
// manifest as an inability to get a builder image = empty string.
func getBuilderImage(f fn.Function) (name string) {
	if f.Build.Builder == builders.S2I {
		name, _ = s2i.BuilderImage(f, builders.S2I)
	} else {
		name, _ = buildpacks.BuilderImage(f, builders.Pack)
	}
	return
}

func getPipelineName(f fn.Function) string {
	var source string
	if f.Build.Git.URL == "" {
		source = "upload"
	} else {
		source = "git"
	}
	return fmt.Sprintf("%s-%s-%s-pipeline", f.Name, f.Build.Builder, source)
}

func getPipelineRunGenerateName(f fn.Function) string {
	return fmt.Sprintf("%s-run-", getPipelineName(f))
}

func getPipelineSecretName(f fn.Function) string {
	return fmt.Sprintf("%s-secret", getPipelineName(f))
}

func getPipelinePvcName(f fn.Function) string {
	return fmt.Sprintf("%s-pvc", getPipelineName(f))
}
