//go:build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	kc := k8s.NewClient(k8s.GetClientConfig())
	removertesting.TestInt_Remove(t,
		keda.NewRemover(true),
		keda.NewDeployer(keda.WithDeployerVerbose(true)),
		keda.NewDescriber(true),
		keda.NewLister(kc, true),
		keda.KedaDeployerName)
}
