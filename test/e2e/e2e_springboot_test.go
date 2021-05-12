// +build e2e_springboot

package e2e

import (
	cli "github.com/boson-project/func/test/cli"
	"testing"
)


func TestDeploySpringboot(t *testing.T) {
	deployScenario := TestDeployScenario{
		FuncName:             "springbootfunc",
		Runtime:              "springboot",
		PerformReadinessTest: true,
		PerformLivenessTest:  true,
		DeploymentValidator:  func(t *testing.T, funcCli *cli.TestShellCli, functionUrl string) {
			// WIP: Currently validated by readiness/liveness probe.
		},
		LongRunTest:          true,
	}
	deployScenario.RunTestScenario(t)
}
