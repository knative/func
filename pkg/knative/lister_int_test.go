//go:build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	kc := k8s.NewClient(k8s.GetClientConfig())
	listertesting.TestInt_List(t,
		knative.NewLister(kc, true),
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewDescriber(true),
		knative.NewRemover(true),
		knative.KnativeDeployerName)
}
