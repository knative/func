//go:build integration

package keda_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	keda "knative.dev/func/pkg/keda"
)

func TestInt_Describe(t *testing.T) {
	kc := defaultKc()
	describertesting.TestInt_Describe(t,
		keda.NewDescriber(kc, true),
		keda.NewDeployer(kc, keda.WithDeployerVerbose(true)),
		keda.NewRemover(kc, true),
		keda.KedaDeployerName)
}
