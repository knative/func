//go:build integration
// +build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/keda"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	listertesting.TestInt_List(t,
		keda.NewLister(true),
		keda.NewDeployer(keda.WithDeployerVerbose(true)),
		keda.NewDescriber(true),
		keda.NewRemover(true),
		keda.KedaDeployerName)
}
