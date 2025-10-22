//go:build integration
// +build integration

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

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		lister.NewLister(false, nil, k8slister.NewGetter(false)),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}
