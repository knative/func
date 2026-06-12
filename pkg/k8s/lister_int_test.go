//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	listertesting "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	kc := defaultKc()
	listertesting.TestInt_List(t,
		k8s.NewLister(kc, true),
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(true)),
		k8s.NewDescriber(kc, true),
		k8s.NewRemover(kc, true),
		k8s.KubernetesDeployerName)
}
