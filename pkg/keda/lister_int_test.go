//go:build integration

package keda_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	kc := k8s.NewClient(k8s.GetClientConfig())
	listertesting.TestInt_List(t,
		keda.NewLister(kc, true),
		keda.NewDeployer(keda.WithDeployerVerbose(true)),
		keda.NewDescriber(true),
		keda.NewRemover(true),
		keda.KedaDeployerName)
}
