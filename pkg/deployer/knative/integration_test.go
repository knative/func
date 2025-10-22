//go:build integration
// +build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	knativedeployer "knative.dev/func/pkg/deployer/knative"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	"knative.dev/func/pkg/lister"
	knativelister "knative.dev/func/pkg/lister/knative"
	knativeremover "knative.dev/func/pkg/remover/knative"
)

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest(t,
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(false)),
		knativeremover.NewRemover(false),
		lister.NewLister(false, knativelister.NewGetter(false), nil),
		knativedescriber.NewDescriber(false),
		deployer.KnativeDeployerName)
}
