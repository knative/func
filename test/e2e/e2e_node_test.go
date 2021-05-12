// +build e2e_node

package e2e

import (
	cli "github.com/boson-project/func/test/cli"
	"github.com/boson-project/func/test/e2e/utils"
	"testing"
)

func TestDeployNode(t *testing.T) {
	deployScenario := TestDeployScenario{
		FuncName:             "nodefunc",
		Runtime:              "node",
		PerformReadinessTest: true,
		PerformLivenessTest:  true,
		DeploymentValidator:  func(t *testing.T, funcCli *cli.TestShellCli, functionUrl string) {
			body, statusCode := utils.HttpGet(t, functionUrl + "?message=hello")
			assert := NewAsserts(t)
			assert.Http2xx(statusCode)
			assert.StringContains(body, `{"message":"hello"}`)
		},
	}
	deployScenario.RunTestScenario(t)
}
