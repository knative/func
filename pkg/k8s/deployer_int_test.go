//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	"knative.dev/func/pkg/k8s"
)

func TestInt_FullPath(t *testing.T) {
	deployertesting.TestInt_FullPath(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewLister(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	deployertesting.TestInt_Deploy(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	deployertesting.TestInt_Metadata(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Events(t *testing.T) {
	t.Skip("Kubernetes deploy does not support func subscribe yet")

	deployertesting.TestInt_Events(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_Scale(t *testing.T) {
	deployertesting.TestInt_Scale(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	deployertesting.TestInt_EnvsUpdate(t,
		k8s.NewDeployer(k8s.WithDeployerVerbose(false)),
		k8s.NewRemover(false),
		k8s.NewDescriber(false),
		k8s.KubernetesDeployerName)
}
