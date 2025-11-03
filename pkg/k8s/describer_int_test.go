//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	describertesting "knative.dev/func/pkg/describer/testing"
	"knative.dev/func/pkg/k8s"
)

func TestInt_Describe(t *testing.T) {
	describertesting.DescribeIntegrationTest(t,
		k8s.NewDescriber(true),
		k8s.NewDeployer(k8s.WithDeployerVerbose(true)),
		k8s.NewRemover(true),
		deployer.KubernetesDeployerName)
}
