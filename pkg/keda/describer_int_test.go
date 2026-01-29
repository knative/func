//go:build integration
// +build integration

package keda_test

import (
	"testing"

	describertesting "knative.dev/func/pkg/describer/testing"
	keda "knative.dev/func/pkg/keda"
)

func TestInt_Describe(t *testing.T) {
	describertesting.TestInt_Describe(t,
		keda.NewDescriber(true),
		keda.NewDeployer(keda.WithDeployerVerbose(true)),
		keda.NewRemover(true),
		keda.KedaDeployerName)
}
