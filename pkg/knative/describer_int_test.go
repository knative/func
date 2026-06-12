//go:build integration

package knative_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	"knative.dev/func/pkg/knative"
)

func TestInt_Describe(t *testing.T) {
	kc := defaultKc()
	describertesting.TestInt_Describe(t,
		knative.NewDescriber(kc, true),
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewRemover(kc, true),
		knative.KnativeDeployerName)
}
