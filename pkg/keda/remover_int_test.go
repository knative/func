//go:build integration
// +build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/keda"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	removertesting.TestInt_Remove(t,
		keda.NewRemover(true),
		keda.NewDeployer(keda.WithDeployerVerbose(true)),
		keda.NewDescriber(true),
		keda.NewLister(true),
		keda.KedaDeployerName)
}
