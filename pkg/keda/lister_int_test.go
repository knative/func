//go:build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/keda"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	kc := defaultKc()
	listertesting.TestInt_List(t,
		keda.NewLister(kc, true),
		keda.NewDeployer(kc, keda.WithDeployerVerbose(true)),
		keda.NewDescriber(kc, true),
		keda.NewRemover(kc, true),
		keda.KedaDeployerName)
}
