//go:build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/keda"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	kc := defaultKc()
	removertesting.TestInt_Remove(t,
		keda.NewRemover(kc, true),
		keda.NewDeployer(kc, keda.WithDeployerVerbose(true)),
		keda.NewDescriber(kc, true),
		keda.NewLister(kc, true),
		keda.KedaDeployerName)
}
