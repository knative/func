//go:build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	"knative.dev/func/pkg/lister"
	knativelister "knative.dev/func/pkg/lister/knative"
	"knative.dev/func/pkg/remover"
	knativeremover "knative.dev/func/pkg/remover/knative"

	knativedeployer "knative.dev/func/pkg/deployer/knative"
)

func TestInt_Remove(t *testing.T) {
	remover.IntegrationTest(t,
		knativeremover.NewRemover(true),
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativedescriber.NewDescriber(true),
		lister.NewLister(true, knativelister.NewGetter(true), nil),
		deployer.KnativeDeployerName)
}
