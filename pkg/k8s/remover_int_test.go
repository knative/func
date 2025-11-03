//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/k8s"
	testing2 "knative.dev/func/pkg/remover/testing"
)

func TestInt_Remove(t *testing.T) {
	testing2.IntegrationTest(t,
		k8s.NewRemover(true),
		k8s.NewDeployer(k8s.WithDeployerVerbose(true)),
		k8s.NewDescriber(true),
		k8s.NewLister(true),
		deployer.KubernetesDeployerName)
}
