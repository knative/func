//go:build e2elc
// +build e2elc

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/testhttp"

	common "knative.dev/func/test/common"
)

// TestFunctionHttpTemplate will invoke a language runtime test against (by default) all supported runtimes.
// The Environment Variable E2E_RUNTIMES can be used to select the languages/runtimes to be tested
// The Environment Variable FUNC_BUILDER can be used to select the builder (s2i or pack).
func TestFunctionCloudEventsTemplate(t *testing.T) {
	var testMatrix = prepareTestMatrix()
	for _, tc := range testMatrix {
		t.Run(fmt.Sprintf("%v_%v_test", tc.Runtime, tc.Builder), func(t *testing.T) {
			lifecycleCloudEventsTest(t, tc.Runtime, tc.Builder)
		})
	}
}

func lifecycleCloudEventsTest(t *testing.T, language string, builder string) {

	var funcName = "cloudevents-function-" + language + "-" + builder
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)

	knFunc.Exec("create", "--language", language, "--template", "cloudevents", funcPath)
	knFunc.Exec("deploy", "--registry", common.GetRegistry(), "--builder", builder, "--path", funcPath)
	defer knFunc.Exec("delete", "--path", funcPath)

	_, functionUrl := common.WaitForFunctionReady(t, funcName)

	validator := ceFuncValidatorMap[language]
	validator.PostCloudEventAndAssert(t, functionUrl)

}

// Basic function responsiveness Validator
type CloudEventsFuncResponsivenessValidator struct {
	//urlMask     string
	contentType string
	bodyData    string
	expects     string
}

func (f *CloudEventsFuncResponsivenessValidator) PostCloudEventAndAssert(t *testing.T, functionUrl string) {

	headers := testhttp.HeaderBuilder().
		AddNonEmpty("Content-Type", f.contentType).
		Add("Ce-Id", "message-1").
		Add("Ce-Type", "HelloMessageType").
		Add("Ce-Source", "test-e2e-lifecycle-test").
		Add("Ce-Specversion", "1.0").Headers

	// push event
	statusCode, funcResponse := testhttp.TestUrl(t, "POST", f.bodyData, functionUrl, headers)

	assert.Assert(t, statusCode == 200)
	if f.expects != "" {
		assert.Assert(t, strings.Contains(funcResponse, f.expects))
	}
}

var ceFuncValidatorMap = map[string]CloudEventsFuncResponsivenessValidator{
	"node": {
		contentType: "text/plain",
		bodyData:    "hello",
		expects:     "",
	},
	"go": {
		contentType: "application/json",
		bodyData:    `{"message": "hello"}`,
		expects:     "",
	},
	"python": {
		contentType: "text/plain",
		bodyData:    "hello",
		expects:     "",
	},
	"quarkus": {
		contentType: "application/json",
		bodyData:    `{"message":"hello"}`,
		expects:     "",
	},
	"springboot": {
		contentType: "text/plain",
		bodyData:    "hello function",
		expects:     "hello function",
	},
	"typescript": {
		contentType: "text/plain",
		bodyData:    "hello",
		expects:     "",
	},
}
