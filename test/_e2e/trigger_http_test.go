package e2e

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

// HTTP Based Function Test Validator
type FunctionHttpResponsivenessValidator struct {
	runtime   string
	targetUrl string
	expects   string
}

func (f FunctionHttpResponsivenessValidator) Validate(t *testing.T, project FunctionTestProject) {
	if f.runtime != project.Runtime {
		return
	}
	if f.targetUrl != "" {
		url := fmt.Sprintf(f.targetUrl, project.FunctionURL)
		body, status := HttpGet(t, url)
		if status != 200 {
			t.Fatalf("Expected status code 200, received %v", status)
		}
		if f.expects != "" && !strings.Contains(body, f.expects) {
			t.Fatalf("Body does not contains expected sentence [%v]", f.expects)
		}
	}
}

var defaultFunctionsHttpValidators = []FunctionHttpResponsivenessValidator{
	{runtime: "node",
		targetUrl: "%s?message=hello",
		expects:   `{"message":"hello"}`,
	},
	{runtime: "go",
		targetUrl: "%s",
		expects:   `OK`,
	},
	{runtime: "python",
		targetUrl: "%s",
		expects:   `Howdy!`,
	},
	{runtime: "quarkus",
		targetUrl: "%s?message=hello",
		expects:   `{"message":"hello"}`,
	},
	{runtime: "springboot",
		targetUrl: "%s/health/readiness",
	},
}

// DefaultFunctionHttpTest is meant to validate the deployed (default) function is actually responsive
func DefaultFunctionHttpTest(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	if project.Template == "http" {
		for _, v := range defaultFunctionsHttpValidators {
			v.Validate(t, project)
		}
	}
}

var newRevisionFunctionsHttpValidators = []FunctionHttpResponsivenessValidator{
	{runtime: "node",
		targetUrl: "%s",
		expects:   `HELLO NODE FUNCTION`,
	},
	{runtime: "go",
		targetUrl: "%s",
		expects:   `HELLO GO FUNCTION`,
	},
	{runtime: "python",
		targetUrl: "%s",
		expects:   `HELLO PYTHON FUNCTION`,
	},
	{runtime: "quarkus",
		targetUrl: "%s",
		expects:   `HELLO QUARKUS FUNCTION`,
	},
}

// NewRevisionFunctionHttpTest is meant to validate the deployed function (new revision from Template) is actually responsive
func NewRevisionFunctionHttpTest(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	if project.IsNewRevision && project.Template == "http" {
		for _, v := range newRevisionFunctionsHttpValidators {
			v.Validate(t, project)
		}
	}
}

// HttpGet Convenient wrapper that calls an URL and returns just the
// body and status code. It fails in case some error occurs in the call
func HttpGet(t *testing.T, url string) (body string, statusCode int) {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Error returned calling %v : %v", url, err.Error())
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err.Error())
	}
	t.Logf("GET %v -> %v", url, resp.Status)
	return string(b), resp.StatusCode
}
