//go:build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/knative"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	kc := defaultKc()
	removertesting.TestInt_Remove(t,
		knative.NewRemover(kc, true),
		knative.NewDeployer(kc, knative.WithDeployerVerbose(true)),
		knative.NewDescriber(kc, true),
		knative.NewLister(kc, true),
		knative.KnativeDeployerName)
}
