//go:build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/knative"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	removertesting.IntegrationTest(t,
		knative.NewRemover(true),
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewDescriber(true),
		knative.NewLister(true),
		deployer.KnativeDeployerName)
}
