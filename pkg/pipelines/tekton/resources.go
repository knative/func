package tekton

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
)

func deletePipelines(ctx context.Context, namespace string, listOptions metav1.ListOptions) (err error) {
	if namespace == "" {
		return errors.New("delete pipeline: namespace required")
	}
	client, err := NewTektonClient(namespace)
	if err != nil {
		return
	}

	return client.Pipelines(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

func deletePipelineRuns(ctx context.Context, namespace string, listOptions metav1.ListOptions) (err error) {
	if namespace == "" {
		return errors.New("delete pipeline run: namespace required")
	}
	client, err := NewTektonClient(namespace)
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
	
	// Kubernetes resource names must be <= 63 characters (RFC 1123)
	// We use a hash-based approach to guarantee uniqueness while staying under the limit
	
	// Create a unique identifier based on function name, builder, and source
	fullIdentifier := fmt.Sprintf("%s-%s-%s", f.Name, f.Build.Builder, source)
	
	// Generate hash of the full identifier
	hash := sha256.Sum256([]byte(fullIdentifier))
	// Use first 8 characters of hex encoding (4 bytes = 8 hex chars)
	shortHash := hex.EncodeToString(hash[:4])
	
	// Format: func-{hash}-{builder}-{source}
	// This gives us: 4 + 1 + 8 + 1 + 3-4 + 1 + 6-3 = 24-26 chars max
	// Well under the 63 char limit with room for future additions
	return fmt.Sprintf("func-%s-%s-%s", shortHash, f.Build.Builder, source)
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
