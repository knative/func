//go:build integration
// +build integration

package knative_test

import (
	"testing"

	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployer/knative"
)

func TestIntegration(t *testing.T) {
	deployer.IntegrationTest(t, knative.NewDeployer(knative.WithDeployerVerbose(false)))
}
