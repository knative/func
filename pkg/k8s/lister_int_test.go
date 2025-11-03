//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	listertesting.IntegrationTest(t,
		k8s.NewLister(true),
		k8s.NewDeployer(k8s.WithDeployerVerbose(true)),
		k8s.NewDescriber(true),
		k8s.NewRemover(true),
		k8s.KubernetesDeployerName)
}
