//go:build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/knative"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	listertesting.IntegrationTest(t,
		knative.NewLister(true),
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewDescriber(true),
		knative.NewRemover(true),
		deployer.KnativeDeployerName)
}
