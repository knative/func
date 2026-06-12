//go:build integration

package k8s_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	"knative.dev/func/pkg/k8s"
)

func TestInt_Describe(t *testing.T) {
	kc := defaultKc()
	describertesting.TestInt_Describe(t,
		k8s.NewDescriber(kc, true),
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(true)),
		k8s.NewRemover(kc, true),
		k8s.KubernetesDeployerName)
}
