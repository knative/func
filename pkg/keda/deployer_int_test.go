//go:build integration
// +build integration

package keda_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	"knative.dev/func/pkg/keda"
)

func TestInt_FullPath(t *testing.T) {
	deployertesting.TestInt_FullPath(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewLister(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	deployertesting.TestInt_Deploy(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	deployertesting.TestInt_Metadata(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}

func TestInt_Events(t *testing.T) {
	t.Skip("Keda deployer does not support func subscribe yet")

	deployertesting.TestInt_Events(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}

func TestInt_Scale(t *testing.T) {
	deployertesting.TestInt_Scale(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	deployertesting.TestInt_EnvsUpdate(t,
		keda.NewDeployer(keda.WithDeployerVerbose(false)),
		keda.NewRemover(false),
		keda.NewDescriber(false),
		keda.KedaDeployerName)
}
