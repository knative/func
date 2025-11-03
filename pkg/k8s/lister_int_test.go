//go:build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	testing2 "knative.dev/func/pkg/lister/testing"
)

func TestInt_List(t *testing.T) {
	testing2.IntegrationTest(t,
		NewLister(true),
		NewDeployer(WithDeployerVerbose(true)),
		NewDescriber(true),
		NewRemover(true),
		deployer.KubernetesDeployerName)
}
