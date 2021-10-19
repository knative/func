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
	runtime           string
	targetUrl         string
	method            string
	contentType       string
	bodyData          string
	expects           string
	responseValidator func(expects string) error
}

func (f FunctionHttpResponsivenessValidator) Validate(t *testing.T, project FunctionTestProject) {
	if f.runtime != project.Runtime || f.targetUrl == "" {
		return
	}

	// Http Invoke Handling
	method := "GET"
	if f.method != "" {
		method = f.method
	}
	url := fmt.Sprintf(f.targetUrl, project.FunctionURL)
	req, err := http.NewRequest(method, url, strings.NewReader(f.bodyData))
	if err != nil {
		t.Fatal(err)
	}
	if f.contentType != "" {
		req.Header.Add("Content-Type", f.contentType)
	}
	client := &http.Client{}
	resp, err := client.Do(req)

	// Http Response Handling
	if err != nil {
		t.Fatalf("Error returned calling %v : %v", url, err.Error())
	}
	defer resp.Body.Close()
	t.Logf("%v %v -> %v", method, url, resp.Status)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err.Error())
	}

	// Assertions
	if resp.StatusCode != 200 {
		t.Fatalf("Expected status code 200, received %v", resp.StatusCode)
	}
	if f.expects != "" && !strings.Contains(string(body), f.expects) {
		t.Fatalf("Body does not contains expected sentence [%v]", f.expects)
	}
	if f.responseValidator != nil {
		if err = f.responseValidator(string(body)); err != nil {
			t.Fatal(err)
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
	{runtime: "typescript",
		targetUrl:   "%s",
		method:      "POST",
		contentType: "application/json",
		bodyData:    `{"message":"hello"}`,
		expects:     `{"message":"hello"}`,
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
	{runtime: "typescript",
		targetUrl: "%s",
		expects:   `HELLO TYPESCRIPT FUNCTION`,
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
