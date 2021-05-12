// +build e2e_go

package e2e

import (
	cli "github.com/boson-project/func/test/cli"
	"github.com/boson-project/func/test/e2e/utils"
	"testing"
)

func TestDeployGo(t *testing.T) {
	deployScenario := TestDeployScenario{
		FuncName:             "gofunc",
		Runtime:              "go",
		PerformReadinessTest: true,
		PerformLivenessTest:  true,
		DeploymentValidator:  func(t *testing.T, funcCli *cli.TestShellCli, functionUrl string) {
			body, statusCode := utils.HttpGet(t, functionUrl)
			assert := NewAsserts(t)
			assert.Http2xx(statusCode)
			assert.StringContains(body, "OK")
		},
	}
	deployScenario.RunTestScenario(t)
}
