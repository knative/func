//go:build integration

package k8s_test

import (
	"testing"

	k8sdeployer "knative.dev/func/pkg/deployer/k8s"
	"knative.dev/func/pkg/describer"
	k8sdescriber "knative.dev/func/pkg/describer/k8s"
	k8sremover "knative.dev/func/pkg/remover/k8s"
)

func TestInt_Describe(t *testing.T) {
	describer.DescribeIntegrationTest(t,
		k8sdescriber.NewDescriber(true),
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(true)),
		k8sremover.NewRemover(true))
}
