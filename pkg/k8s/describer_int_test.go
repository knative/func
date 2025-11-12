//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	"knative.dev/func/pkg/k8s"
)

func TestInt_Describe(t *testing.T) {
	describertesting.TestInt_Describe(t,
		k8s.NewDescriber(true),
		k8s.NewDeployer(k8s.WithDeployerVerbose(true)),
		k8s.NewRemover(true),
		k8s.KubernetesDeployerName)
}
