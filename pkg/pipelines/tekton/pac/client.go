package pac

import (
	"fmt"

	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/generated/clientset/versioned/typed/pipelinesascode/v1alpha1"

	"knative.dev/func/pkg/k8s"
)

func NewTektonPacClientAndResolvedNamespace(defaultNamespace string) (*pacv1alpha1.PipelinesascodeV1alpha1Client, string, error) {
	namespace, err := k8s.GetNamespace(defaultNamespace)
	if err != nil {
		return nil, "", err
	}

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new tekton pac client: %w", err)
	}

	client, err := pacv1alpha1.NewForConfig(restConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new tekton pac client: %v", err)
	}

	return client, namespace, nil
}
