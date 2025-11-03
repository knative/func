//go:build integration
// +build integration

package knative_test

import (
	"testing"

	deployertesting "knative.dev/func/pkg/deployer/testing"
	"knative.dev/func/pkg/knative"
)

func TestIntegration(t *testing.T) {
	deployertesting.IntegrationTest_FullPath(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(true),
		knative.NewLister(true),
		knative.NewDescriber(true),
		knative.KnativeDeployerName)
}

func TestInt_Deploy(t *testing.T) {
	deployertesting.IntegrationTest_Deploy(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(false),
		knative.NewDescriber(false),
		knative.KnativeDeployerName)
}

func TestInt_Metadata(t *testing.T) {
	deployertesting.IntegrationTest_Metadata(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(false),
		knative.NewDescriber(false),
		knative.KnativeDeployerName)
}

func TestInt_Events(t *testing.T) {
	deployertesting.IntegrationTest_Events(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(false),
		knative.NewDescriber(false),
		knative.KnativeDeployerName)
}

func TestInt_Scale(t *testing.T) {
	deployertesting.IntegrationTest_Scale(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(false),
		knative.NewDescriber(false),
		knative.KnativeDeployerName)
}

func TestInt_EnvsUpdate(t *testing.T) {
	deployertesting.IntegrationTest_EnvsUpdate(t,
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(false),
		knative.NewDescriber(false),
		knative.KnativeDeployerName)
}
