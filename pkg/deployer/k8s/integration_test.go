//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	k8sdeployer "knative.dev/func/pkg/deployer/k8s"
	k8sdescriber "knative.dev/func/pkg/describer/k8s"
	k8slister "knative.dev/func/pkg/lister/k8s"
	k8sremover "knative.dev/func/pkg/remover/k8s"
)

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest_FullPath(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8slister.NewLister(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}

func TestIntegration_Deploy(t *testing.T) {
	deployer.IntegrationTest_Deploy(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}

func TestIntegration_Metadata(t *testing.T) {
	deployer.IntegrationTest_Metadata(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}

func TestIntegration_Events(t *testing.T) {
	deployer.IntegrationTest_Events(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}

func TestIntegration_Scale(t *testing.T) {
	deployer.IntegrationTest_Scale(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}

func TestIntegration_EnvsUpdate(t *testing.T) {
	deployer.IntegrationTest_EnvsUpdate(t,
		k8sdeployer.NewDeployer(k8sdeployer.WithDeployerVerbose(false)),
		k8sremover.NewRemover(false),
		k8sdescriber.NewDescriber(false),
		deployer.KubernetesDeployerName)
}
