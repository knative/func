package e2e

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"knative.dev/kn-plugin-func/k8s"

	"testing"
)

// RetrieveKnativeServiceResource wraps the logic to query knative serving resources in current namespace
func RetrieveKnativeServiceResource(t *testing.T, serviceName string) *unstructured.Unstructured {
	// create k8s dynamic client
	config, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err.Error())
	}
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err.Error())
	}

	knativeServiceResource := schema.GroupVersionResource{
		Group:    "serving.knative.dev",
		Version:  "v1",
		Resource: "services",
	}
	namespace, _, _ := k8s.GetClientConfig().Namespace()
	resource, err := dynClient.Resource(knativeServiceResource).Namespace(namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	return resource
}

// GetCurrentServiceRevision retrieves current revision name for the deployed function
func GetCurrentServiceRevision(t *testing.T, project *FunctionTestProject) string {
	resource := RetrieveKnativeServiceResource(t, project.FunctionName)
	rootMap := resource.UnstructuredContent()
	statusMap := rootMap["status"].(map[string]interface{})
	latestReadyRevision := statusMap["latestReadyRevisionName"].(string)
	return latestReadyRevision
}
