//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	"knative.dev/func/pkg/k8s"
)

func TestIntegration(t *testing.T) {
	deployertesting.IntegrationTest_FullPath(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewLister(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	deployertesting.IntegrationTest_Deploy(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	deployertesting.IntegrationTest_Metadata(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Events(t *testing.T) {
	deployertesting.IntegrationTest_Events(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Scale(t *testing.T) {
	deployertesting.IntegrationTest_Scale(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	deployertesting.IntegrationTest_EnvsUpdate(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}
