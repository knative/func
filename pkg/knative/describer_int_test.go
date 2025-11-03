//go:build integration

package knative_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	"knative.dev/func/pkg/knative"
)

func TestInt_Describe(t *testing.T) {
	describertesting.DescribeIntegrationTest(t,
		knative.NewDescriber(true),
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewRemover(true),
		knative.KnativeDeployerName)
}
