package e2e

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

type SimpleTestEvent struct {
	Type        string
	Source      string
	ContentType string
	Data        string
}

func (s SimpleTestEvent) pushTo(url string, t *testing.T) (body string, statusCode int, err error) {
	client := &http.Client{Timeout: time.Second * 15}
	req, err := http.NewRequest("POST", url, strings.NewReader(s.Data))
	req.Header.Add("Ce-Id", "message-1")
	req.Header.Add("Ce-Specversion", "1.0")
	req.Header.Add("Ce-Type", s.Type)
	req.Header.Add("Ce-Source", s.Source)
	req.Header.Add("Content-Type", s.ContentType)
	resp, err := client.Do(req)

	if err != nil {
		return "", 0, err
	}
	t.Logf("event POST %v -> %v", url, resp.Status)
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("Error reading response body: %v", err.Error())
	}
	return string(b), resp.StatusCode, nil
}

type FunctionCloudEventsValidatorEntry struct {
	targetUrl   string
	contentType string
	data        string
	expectsBody string
}

var defaultFunctionsCloudEventsValidators = map[string]FunctionCloudEventsValidatorEntry{
	"quarkus": {
		targetUrl:   "%s",
		contentType: "application/json",
		data:        `{"message":"hello"}`,
	},
	"springboot": {
		targetUrl:   "%s/echo",
		contentType: "application/json",
		data:        `hello function`,
		expectsBody: "hello function",
	},
}

// DefaultFunctionEventsTest executes a common test (applied for all runtimes) against a deployed
// functions that responds to CloudEvents
func DefaultFunctionEventsTest(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	if project.Template == "cloudevents" && project.IsDeployed {

		simpleEvent := SimpleTestEvent{
			Type:        "e2e.test",
			Source:      "e2e:test",
			ContentType: "text/plain",
			Data:        "hello",
		}
		targetUrl := project.FunctionURL

		// Some runtime
		customData, ok := defaultFunctionsCloudEventsValidators[project.Runtime]
		if ok {
			simpleEvent.Data = customData.data
			simpleEvent.ContentType = customData.contentType
			targetUrl = fmt.Sprintf(customData.targetUrl, project.FunctionURL)
		}

		body, statusCode, err := simpleEvent.pushTo(targetUrl, t)
		if err != nil {
			t.Fatal(err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status code 200, received %v", statusCode)
		}
		if customData.expectsBody != "" && !strings.Contains(body, customData.expectsBody) {
			t.Fatalf("Body does not contains expected sentence [%v]", customData.expectsBody)
		}

	} else {
		t.Fatalf("Expected e2e cloudevents test to run")
	}

}
