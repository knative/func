//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	k8sdeployer "knative.dev/func/pkg/deployer/k8s"
	k8sdescriber "knative.dev/func/pkg/describer/k8s"
	"knative.dev/func/pkg/lister"
	k8slister "knative.dev/func/pkg/lister/k8s"
	k8sremover "knative.dev/func/pkg/remover/k8s"
)

func TestInt_List(t *testing.T) {
	lister.IntegrationTest(t,
		k8slister.NewLister(true),
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(true)),
		k8sdescriber.NewDescriber(true),
		k8sremover.NewRemover(true),
		deployer.KubernetesDeployerName)
}
