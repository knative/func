//go:build integration
// +build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/knative"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	listertesting.TestInt_List(t,
		knative.NewLister(true),
		knative.NewDeployer(knative.WithDeployerVerbose(true)),
		knative.NewDescriber(true),
		knative.NewRemover(true),
		knative.KnativeDeployerName)
}
