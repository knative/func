//go:build integration

package keda_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
)

func TestInt_FullPath(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_FullPath(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewLister(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_Deploy(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_Metadata(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_Events(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	t.Skip("Keda deployer does not support func subscribe yet")

	deployertesting.TestInt_Events(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_Scale(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_Scale(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_EnvsUpdate(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_ResourceValidationOnFirstDeploy(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_ResourceValidationOnFirstDeploy(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}

func TestInt_OperatorSync(t *testing.T) {
	kc := k8s.NewClientFromKubeconfig()
	deployertesting.TestInt_OperatorSync(t,
		keda.NewDeployer(kc, keda.WithDeployerVerbose(false)),
		keda.NewRemover(kc, false),
		keda.NewDescriber(kc, false),
		keda.KedaDeployerName)
}
