//go:build integration
// +build integration

package k8s_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployer/k8s"
)

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest(t, k8s.NewDeployer(k8s.WithDeployerVerbose(false)))
}
