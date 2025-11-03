//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	k8sdescriber "knative.dev/func/pkg/describer/k8s"
	k8slister "knative.dev/func/pkg/lister/k8s"
	"knative.dev/func/pkg/remover"
	k8sremover "knative.dev/func/pkg/remover/k8s"

	k8sdeployer "knative.dev/func/pkg/deployer/k8s"
)

func TestInt_Remove(t *testing.T) {
	remover.IntegrationTest(t,
		k8sremover.NewRemover(true),
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(true)),
		k8sdescriber.NewDescriber(true),
		k8slister.NewLister(true),
		deployer.KubernetesDeployerName)
}
