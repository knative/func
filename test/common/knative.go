package common

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"knative.dev/func/pkg/k8s"

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
func GetCurrentServiceRevision(t *testing.T, serviceName string) string {
	resource := RetrieveKnativeServiceResource(t, serviceName)
	rootMap := resource.UnstructuredContent()
	statusMap := rootMap["status"].(map[string]interface{})
	latestReadyRevision := statusMap["latestReadyRevisionName"].(string)
	return latestReadyRevision
}

func GetKnativeServiceRevisionAndUrl(t *testing.T, serviceName string) (revision string, url string) {
	t.Helper()
	var ok bool
	resource := RetrieveKnativeServiceResource(t, serviceName)
	rootMap := resource.UnstructuredContent()
	statusMap, ok := rootMap["status"].(map[string]interface{})
	if !ok {
		t.Fatal("absent status")
	}
	revision, ok = statusMap["latestReadyRevisionName"].(string)
	if !ok {
		t.Fatal("absent ready revision")
	}
	url, ok = statusMap["url"].(string)
	if !ok {
		t.Fatal("absent url")
	}
	return revision, url
}
