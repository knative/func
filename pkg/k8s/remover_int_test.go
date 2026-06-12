//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	kc := defaultKc()
	removertesting.TestInt_Remove(t,
		k8s.NewRemover(kc, true),
		k8s.NewDeployer(kc, k8s.WithDeployerVerbose(true)),
		k8s.NewDescriber(kc, true),
		k8s.NewLister(kc, true),
		k8s.KubernetesDeployerName)
}
