//go:build integration

package knative_test

import (
	"testing"

	knativedeployer "knative.dev/func/pkg/deployer/knative"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	"knative.dev/func/pkg/lister"
	k8slister "knative.dev/func/pkg/lister/k8s"
	knativelister "knative.dev/func/pkg/lister/knative"
	knativeremover "knative.dev/func/pkg/remover/knative"
)

func TestInt_List(t *testing.T) {
	lister.IntegrationTest(t,
		lister.NewLister(true,
			knativelister.NewGetter(true),
			k8slister.NewGetter(true)),
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativedescriber.NewDescriber(true),
		knativeremover.NewRemover(true))
}
