//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/k8s"
	removertesting "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	removertesting.TestInt_Remove(t,
		k8s.NewRemover(true),
		k8s.NewDeployer(k8s.WithDeployerVerbose(true)),
		k8s.NewDescriber(true),
		k8s.NewLister(true),
		k8s.KubernetesDeployerName)
}
