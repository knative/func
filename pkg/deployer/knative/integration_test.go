//go:build integration
// +build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	knativedeployer "knative.dev/func/pkg/deployer/knative"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	knativelister "knative.dev/func/pkg/lister/knative"
	knativeremover "knative.dev/func/pkg/remover/knative"
)

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest_FullPath(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(true),
		knativelister.NewLister(true),
		knativedescriber.NewDescriber(true),
		deployer.KnativeDeployerName)
}

func TestIntegration_Deploy(t *testing.T) {
	deployer.IntegrationTest_Deploy(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(false),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}

func TestIntegration_Metadata(t *testing.T) {
	deployer.IntegrationTest_Metadata(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(false),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}

func TestIntegration_Events(t *testing.T) {
	deployer.IntegrationTest_Events(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(false),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}

func TestIntegration_Scale(t *testing.T) {
	deployer.IntegrationTest_Scale(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(false),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}

func TestIntegration_EnvsUpdate(t *testing.T) {
	deployer.IntegrationTest_EnvsUpdate(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(false),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}
