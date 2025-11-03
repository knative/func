//go:build integration

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

func TestInt_List(t *testing.T) {
	lister.IntegrationTest(t,
		knativelister.NewLister(true),
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativedescriber.NewDescriber(true),
		knativeremover.NewRemover(true),
		deployer.KnativeDeployerName)
}
