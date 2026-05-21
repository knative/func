package keda

import (
	"fmt"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	"knative.dev/func/pkg/k8s"
)

func NewHTTPScaledObjectClientset(kc *k8s.Client) (*httpv1alpha1.Clientset, error) {
	restConfig, err := kc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get clientconfig: %w", err)
	}

	return httpv1alpha1.NewForConfig(restConfig)
}
