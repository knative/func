// +build e2e_python

package e2e

import (
	cli "github.com/boson-project/func/test/cli"
	"github.com/boson-project/func/test/e2e/utils"
	"testing"
)

func TestDeployPython(t *testing.T) {
	deployScenario := TestDeployScenario{
		FuncName:             "pythonfunc",
		Runtime:              "python",
		PerformReadinessTest: true,
		PerformLivenessTest:  true,
		DeploymentValidator:  func(t *testing.T, funcCli *cli.TestShellCli, functionUrl string) {
			body, statusCode := utils.HttpGet(t, functionUrl)
			assert := NewAsserts(t)
			assert.Http2xx(statusCode)
			assert.StringContains(body, "Howdy!")
		},
	}
	deployScenario.RunTestScenario(t)
}