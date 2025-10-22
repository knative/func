//go:build integration

package knative_test

import (
	"testing"

	knativedeployer "knative.dev/func/pkg/deployer/knative"
	"knative.dev/func/pkg/describer"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	knativeremover "knative.dev/func/pkg/remover/knative"
)

func TestInt_Describe(t *testing.T) {
	describer.DescribeIntegrationTest(t,
		knativedescriber.NewDescriber(true),
		knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true)),
		knativeremover.NewRemover(true))
}
